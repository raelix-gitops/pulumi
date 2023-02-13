// Copyright 2016-2023, Pulumi Corporation.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package filestate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gofrs/uuid"

	user "github.com/tweekmonster/luser"
	"gocloud.dev/blob"
	_ "gocloud.dev/blob/azureblob" // driver for azblob://
	_ "gocloud.dev/blob/fileblob"  // driver for file://
	"gocloud.dev/blob/gcsblob"     // driver for gs://
	_ "gocloud.dev/blob/s3blob"    // driver for s3://
	"gocloud.dev/gcerrors"

	"github.com/pulumi/pulumi/pkg/v3/authhelpers"
	"github.com/pulumi/pulumi/pkg/v3/backend"
	"github.com/pulumi/pulumi/pkg/v3/backend/display"
	"github.com/pulumi/pulumi/pkg/v3/engine"
	"github.com/pulumi/pulumi/pkg/v3/operations"
	"github.com/pulumi/pulumi/pkg/v3/resource/deploy"
	"github.com/pulumi/pulumi/pkg/v3/resource/edit"
	"github.com/pulumi/pulumi/pkg/v3/resource/stack"
	"github.com/pulumi/pulumi/pkg/v3/secrets"
	"github.com/pulumi/pulumi/pkg/v3/util/validation"
	"github.com/pulumi/pulumi/sdk/v3/go/common/apitype"
	"github.com/pulumi/pulumi/sdk/v3/go/common/diag"
	"github.com/pulumi/pulumi/sdk/v3/go/common/diag/colors"
	sdkDisplay "github.com/pulumi/pulumi/sdk/v3/go/common/display"
	"github.com/pulumi/pulumi/sdk/v3/go/common/encoding"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource/config"
	"github.com/pulumi/pulumi/sdk/v3/go/common/tokens"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/cmdutil"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/contract"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/result"
	"github.com/pulumi/pulumi/sdk/v3/go/common/workspace"
	"gopkg.in/yaml.v3"
)

// PulumiFilestateGzipEnvVar is an env var that must be truthy
// to enable gzip compression when using the filestate backend.
const PulumiFilestateGzipEnvVar = "PULUMI_SELF_MANAGED_STATE_GZIP"

// Backend extends the base backend interface with specific information about local backends.
type Backend interface {
	backend.Backend
	local() // at the moment, no local specific info, so just use a marker function.

	// Upgrade to the latest state store version
	Upgrade(ctx context.Context) error
}

type localBackend struct {
	d diag.Sink

	// originalURL is the URL provided when the localBackend was initialized, for example
	// "file://~". url is a canonicalized version that should be used when persisting data.
	// (For example, replacing ~ with the home directory, making an absolute path, etc.)
	originalURL string
	url         string

	bucket Bucket
	mutex  sync.Mutex

	lockID string

	gzip bool

	// true if this backend is in project mode. This changes where stack files are read/written and how stack
	// references are parsed.
	projectMode bool
}

type localBackendReference struct {
	name    tokens.Name
	project tokens.Name
}

func (r *localBackendReference) String() string {
	return r.FullyQualifiedName().String()
}

func (r *localBackendReference) Name() tokens.Name {
	return r.name
}

func (r *localBackendReference) Project() tokens.Name {
	return r.project
}

func (r *localBackendReference) FullyQualifiedName() tokens.QName {
	if r.project == "" {
		return r.name.Q()
	}
	return tokens.QName(fmt.Sprintf("organization/%s/%s", r.project, r.name))
}

func IsFileStateBackendURL(urlstr string) bool {
	u, err := url.Parse(urlstr)
	if err != nil {
		return false
	}

	return blob.DefaultURLMux().ValidBucketScheme(u.Scheme)
}

const FilePathPrefix = "file://"

type pulumiState struct {
	// Version is the current version of the state store
	Version int `json:"version,omitempty" yaml:"version,omitempty"`
}

func New(ctx context.Context, d diag.Sink, originalURL string) (Backend, error) {
	if !IsFileStateBackendURL(originalURL) {
		return nil, fmt.Errorf("local URL %s has an illegal prefix; expected one of: %s",
			originalURL, strings.Join(blob.DefaultURLMux().BucketSchemes(), ", "))
	}

	u, err := massageBlobPath(originalURL)
	if err != nil {
		return nil, err
	}

	p, err := url.Parse(u)
	if err != nil {
		return nil, err
	}

	blobmux := blob.DefaultURLMux()

	// for gcp we want to support additional credentials
	// schemes on top of go-cloud's default credentials mux.
	if p.Scheme == gcsblob.Scheme {
		blobmux, err = authhelpers.GoogleCredentialsMux(ctx)
		if err != nil {
			return nil, err
		}
	}

	bucket, err := blobmux.OpenBucket(ctx, u)
	if err != nil {
		return nil, fmt.Errorf("unable to open bucket %s: %w", u, err)
	}

	if !strings.HasPrefix(u, FilePathPrefix) {
		bucketSubDir := strings.TrimLeft(p.Path, "/")
		if bucketSubDir != "" {
			if !strings.HasSuffix(bucketSubDir, "/") {
				bucketSubDir += "/"
			}

			bucket = blob.PrefixedBucket(bucket, bucketSubDir)
		}
	}

	// Check if there is a .pulumi/Pulumi.yaml file in the bucket
	b := &wrappedBucket{bucket: bucket}
	pulumiYamlPath := filepath.Join(workspace.BookkeepingDir, "Pulumi.yaml")
	pulumiYaml, err := b.ReadAll(ctx, pulumiYamlPath)
	if err != nil {
		if gcerrors.Code(err) != gcerrors.NotFound {
			return nil, fmt.Errorf("could not read 'Pulumi.yaml': %w", err)
		}
	}

	var pulumiState pulumiState
	if err != nil {
		// We'll only get here if err is NotFound, at this point we want to see if this is a fresh new store,
		// in which case we'll write the new Pulumi.yaml, or if there's existing data here we'll fallback to
		// non-project mode.
		bucketIter := b.bucket.List(&blob.ListOptions{
			Delimiter: "/",
			Prefix:    workspace.BookkeepingDir,
		})
		_, err := bucketIter.Next(ctx)
		if err == io.EOF {
			// It's an empty bucket, turn on project mode
			pulumiState.Version = 1
			pulumiYaml, err = yaml.Marshal(&pulumiState)
			contract.AssertNoErrorf(err, "Could not marshal filestate.pulumiState to yaml")
			err := b.WriteAll(ctx, pulumiYamlPath, pulumiYaml, nil)
			if err != nil {
				return nil, fmt.Errorf("could not write 'Pulumi.yaml': %w", err)
			}
		}
	} else {
		err = yaml.Unmarshal(pulumiYaml, &pulumiState)
		if err != nil {
			return nil, fmt.Errorf("state store corrupted, could not unmarshal 'Pulumi.yaml': %w", err)
		}
		if pulumiState.Version < 1 {
			return nil, fmt.Errorf("state store corrupted, 'Pulumi.yaml' reports an invalid version of %d", pulumiState.Version)
		}
		if pulumiState.Version > 1 {
			return nil, fmt.Errorf(
				"state store unsupported, 'Pulumi.yaml' reports an version of %d unsupported by this version of pulumi",
				pulumiState.Version)
		}
	}

	// Allocate a unique lock ID for this backend instance.
	lockID, err := uuid.NewV4()
	if err != nil {
		return nil, err
	}

	gzipCompression := cmdutil.IsTruthy(os.Getenv(PulumiFilestateGzipEnvVar))

	backend := &localBackend{
		d:           d,
		originalURL: originalURL,
		url:         u,
		bucket:      b,
		lockID:      lockID.String(),
		gzip:        gzipCompression,
		projectMode: pulumiState.Version != 0,
	}

	// If we're in project mode warn about any old stack files
	if backend.projectMode {
		files, err := listBucket(b, backend.stackPath(nil))
		// If there's an error listing don't fail, just don't print the warnings
		if err == nil {
			for _, file := range files {
				if !file.IsDir {
					objName := objectName(file)
					// Skip files without valid extensions (e.g., *.bak files).
					ext := filepath.Ext(objName)
					// But accept gzip compression
					if ext == encoding.GZIPExt {
						objName = strings.TrimSuffix(objName, encoding.GZIPExt)
						ext = filepath.Ext(objName)
					}

					if _, has := encoding.Marshalers[ext]; !has {
						continue
					}

					// This looks like a stack file! Warn about it
					name := objName[:len(objName)-len(ext)]
					d.Warningf(&diag.Diag{
						Message: "Found legacy stack file '%s', you should run 'pulumi state migrate'",
					}, name)
				}
			}
		}
	}

	return backend, nil
}

func (b *localBackend) Upgrade(ctx context.Context) error {
	files, err := listBucket(b.bucket, b.stackPath(nil))
	if err != nil {
		return err
	}
	for _, file := range files {
		if !file.IsDir {
			objName := objectName(file)
			// Skip files without valid extensions (e.g., *.bak files).
			ext := filepath.Ext(objName)
			// But accept gzip compression
			if ext == encoding.GZIPExt {
				objName = strings.TrimSuffix(objName, encoding.GZIPExt)
				ext = filepath.Ext(objName)
			}

			if _, has := encoding.Marshalers[ext]; !has {
				continue
			}

			// This looks like a stack file! Move it to the right project folder
			name := tokens.Name(objName[:len(objName)-len(ext)])
			// make an old style stack ref
			old := &localBackendReference{name: name}

			chk, err := b.getCheckpoint(old)
			if err != nil {
				return err
			}
			// Try and find the project name from _any_ resource URN
			var project tokens.Name
			if chk.Latest != nil {
				for _, res := range chk.Latest.Resources {
					project = tokens.Name(res.URN.Project())
					break
				}
			}
			if project == "" {
				return fmt.Errorf("could not determine project for stack file %s", objName)
			}

			new := &localBackendReference{name: name, project: project}
			err = b.renameStack(ctx, old, new)
			if err != nil {
				return err
			}
		}
	}

	var pulumiState pulumiState
	pulumiState.Version = 1
	pulumiYaml, err := yaml.Marshal(&pulumiState)
	contract.AssertNoErrorf(err, "Could not marshal filestate.pulumiState to yaml")
	err = b.bucket.WriteAll(ctx, "Pulumi.yaml", pulumiYaml, nil)
	if err != nil {
		return fmt.Errorf("could not write 'Pulumi.yaml': %w", err)
	}
	b.projectMode = true

	return nil
}

// massageBlobPath takes the path the user provided and converts it to an appropriate form go-cloud
// can support.  Importantly, s3/azblob/gs paths should not be be touched. This will only affect
// file:// paths which have a few oddities around them that we want to ensure work properly.
func massageBlobPath(path string) (string, error) {
	if !strings.HasPrefix(path, FilePathPrefix) {
		// Not a file:// path.  Keep this untouched and pass directly to gocloud.
		return path, nil
	}

	// Strip off the "file://" portion so we can examine and determine what to do with the rest.
	path = strings.TrimPrefix(path, FilePathPrefix)

	// We need to specially handle ~.  The shell doesn't take care of this for us, and later
	// functions we run into can't handle this either.
	//
	// From https://stackoverflow.com/questions/17609732/expand-tilde-to-home-directory
	if strings.HasPrefix(path, "~") {
		usr, err := user.Current()
		if err != nil {
			return "", fmt.Errorf("Could not determine current user to resolve `file://~` path.: %w", err)
		}

		if path == "~" {
			path = usr.HomeDir
		} else {
			path = filepath.Join(usr.HomeDir, path[2:])
		}
	}

	// For file:// backend, ensure a relative path is resolved. fileblob only supports absolute paths.
	path, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("An IO error occurred while building the absolute path: %w", err)
	}

	// Using example from https://godoc.org/gocloud.dev/blob/fileblob#example-package--OpenBucket
	// On Windows, convert "\" to "/" and add a leading "/". (See https://gocloud.dev/howto/blob/#local)
	path = filepath.ToSlash(path)
	if os.PathSeparator != '/' && !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	return FilePathPrefix + path, nil
}

func Login(ctx context.Context, d diag.Sink, url string) (Backend, error) {
	be, err := New(ctx, d, url)
	if err != nil {
		return nil, err
	}
	return be, workspace.StoreAccount(be.URL(), workspace.Account{}, true)
}

func (b *localBackend) getReference(ref backend.StackReference) (*localBackendReference, error) {
	localStackRef, is := ref.(*localBackendReference)
	if !is {
		return nil, fmt.Errorf("bad stack reference type")
	}
	if localStackRef.project == "" && b.projectMode {
		return nil, fmt.Errorf("bad stack reference, project was not set")
	}
	if localStackRef.project != "" && !b.projectMode {
		return nil, fmt.Errorf("bad stack reference, project was set")
	}
	return localStackRef, nil
}

func (b *localBackend) local() {}

func (b *localBackend) Name() string {
	name, err := os.Hostname()
	contract.IgnoreError(err)
	if name == "" {
		name = "local"
	}
	return name
}

func (b *localBackend) URL() string {
	return b.originalURL
}

func (b *localBackend) StateDir() string {
	return workspace.BookkeepingDir
}

func (b *localBackend) GetPolicyPack(ctx context.Context, policyPack string,
	d diag.Sink) (backend.PolicyPack, error) {

	return nil, fmt.Errorf("File state backend does not support resource policy")
}

func (b *localBackend) ListPolicyGroups(ctx context.Context, orgName string, _ backend.ContinuationToken) (
	apitype.ListPolicyGroupsResponse, backend.ContinuationToken, error) {
	return apitype.ListPolicyGroupsResponse{}, nil, fmt.Errorf("File state backend does not support resource policy")
}

func (b *localBackend) ListPolicyPacks(ctx context.Context, orgName string, _ backend.ContinuationToken) (
	apitype.ListPolicyPacksResponse, backend.ContinuationToken, error) {
	return apitype.ListPolicyPacksResponse{}, nil, fmt.Errorf("File state backend does not support resource policy")
}

func (b *localBackend) SupportsTags() bool {
	return false
}

func (b *localBackend) SupportsOrganizations() bool {
	return false
}

func (b *localBackend) ParseStackReference(stackRef string) (backend.StackReference, error) {
	return b.parseStackReference(stackRef)
}

func (b *localBackend) parseStackReference(stackRef string) (*localBackendReference, error) {
	if b.projectMode {
		var name, project, org string
		split := strings.Split(stackRef, "/")
		switch len(split) {
		case 1:
			name = split[0]
		case 2:
			org = split[0]
			name = split[1]
		case 3:
			org = split[0]
			project = split[1]
			name = split[2]
		default:
			return nil, fmt.Errorf("could not parse stack reference '%s'", stackRef)
		}

		// If the provided stack name didn't include the org or project, infer them from the local
		// environment.
		if org == "" {
			// Filestate organization MUST always be "organization"
			org = "organization"
		}

		if org != "organization" {
			return nil, errors.New("organization name must be 'organization'")
		}

		if project == "" {
			currentProject, projectErr := workspace.DetectProject()
			if projectErr != nil {
				return nil, fmt.Errorf("if you're using the --stack flag, "+
					"pass the fully qualified name (organization/project/stack): %w", projectErr)
			}

			project = currentProject.Name.String()
		}

		if len(project) > 100 {
			return nil, errors.New("project names must be less than 100 characters")
		}

		if project != "" && !tokens.IsName(project) {
			return nil, fmt.Errorf(
				"project names may only contain alphanumerics, hyphens, underscores, and periods: %s",
				project)
		}

		if !tokens.IsName(name) || len(name) > 100 {
			return nil, fmt.Errorf(
				"stack names are limited to 100 characters and may only contain alphanumeric, hyphens, underscores, or periods: %s",
				name)
		}

		return &localBackendReference{name: tokens.Name(name), project: tokens.Name(project)}, nil
	}

	if !tokens.IsName(stackRef) || len(stackRef) > 100 {
		return nil, fmt.Errorf(
			"stack names are limited to 100 characters and may only contain alphanumeric, hyphens, underscores, or periods: %s",
			stackRef)
	}
	return &localBackendReference{name: tokens.Name(stackRef)}, nil
}

// ValidateStackName verifies the stack name is valid for the local backend.
func (b *localBackend) ValidateStackName(stackRef string) error {
	_, err := b.ParseStackReference(stackRef)
	return err
}

func (b *localBackend) DoesProjectExist(ctx context.Context, projectName string) (bool, error) {
	projects, err := b.getLocalProjects()
	if err != nil {
		return false, err
	}

	for _, project := range projects {
		if string(project) == projectName {
			return true, nil
		}
	}

	return false, nil
}

// Confirm the specified stack's project doesn't contradict the Pulumi.yaml of the current project. If the CWD
// is not in a Pulumi project, does not contradict. If the project name in Pulumi.yaml is "foo", a stack with a
// name of bar/foo should not work.
func currentProjectContradictsWorkspace(stack *localBackendReference) bool {
	contract.Requiref(stack != nil, "stack", "is nil")

	if stack.project == "" {
		return false
	}

	projPath, err := workspace.DetectProjectPath()
	if err != nil {
		return false
	}

	if projPath == "" {
		return false
	}

	proj, err := workspace.LoadProject(projPath)
	if err != nil {
		return false
	}

	return proj.Name.String() != stack.project.String()
}

func (b *localBackend) CreateStack(ctx context.Context, stackRef backend.StackReference,
	opts interface{}) (backend.Stack, error) {
	localStackRef, err := b.getReference(stackRef)
	if err != nil {
		return nil, err
	}

	err = b.Lock(ctx, stackRef)
	if err != nil {
		return nil, err
	}
	defer b.Unlock(ctx, stackRef)

	if currentProjectContradictsWorkspace(localStackRef) {
		return nil, fmt.Errorf("provided project name %q doesn't match Pulumi.yaml", localStackRef.project)
	}

	contract.Requiref(opts == nil, "opts", "local stacks do not support any options")

	stackName := localStackRef.FullyQualifiedName()
	if stackName == "" {
		return nil, errors.New("invalid empty stack name")
	}

	if _, _, err := b.getStack(ctx, localStackRef); err == nil {
		return nil, &backend.StackAlreadyExistsError{StackName: string(stackName)}
	}

	tags, err := backend.GetEnvironmentTagsForCurrentStack()
	if err != nil {
		return nil, fmt.Errorf("getting stack tags: %w", err)
	}
	if err = validation.ValidateStackProperties(stackName.Name().String(), tags); err != nil {
		return nil, fmt.Errorf("validating stack properties: %w", err)
	}

	file, err := b.saveStack(localStackRef, nil, nil)
	if err != nil {
		return nil, err
	}

	stack := newStack(localStackRef, file, nil, b)
	fmt.Printf("Created stack '%s'\n", stack.Ref())

	return stack, nil
}

func (b *localBackend) GetStack(ctx context.Context, stackRef backend.StackReference) (backend.Stack, error) {
	localStackRef, err := b.getReference(stackRef)
	if err != nil {
		return nil, err
	}

	snapshot, path, err := b.getStack(ctx, localStackRef)

	switch {
	case gcerrors.Code(err) == gcerrors.NotFound:
		return nil, nil
	case err != nil:
		return nil, err
	default:
		return newStack(localStackRef, path, snapshot, b), nil
	}
}

func (b *localBackend) ListStacks(
	ctx context.Context, _ backend.ListStacksFilter, _ backend.ContinuationToken) (
	[]backend.StackSummary, backend.ContinuationToken, error) {
	stacks, err := b.getLocalStacks()
	if err != nil {
		return nil, nil, err
	}

	// Note that the provided stack filter is not honored, since fields like
	// organizations and tags aren't persisted in the local backend.
	var results = make([]backend.StackSummary, 0, len(stacks))
	for _, stackRef := range stacks {
		chk, err := b.getCheckpoint(stackRef)
		if err != nil {
			return nil, nil, err
		}
		results = append(results, newLocalStackSummary(stackRef, chk))
	}

	return results, nil, nil
}

func (b *localBackend) RemoveStack(ctx context.Context, stack backend.Stack, force bool) (bool, error) {
	localStackRef, err := b.getReference(stack.Ref())
	if err != nil {
		return false, err
	}

	err = b.Lock(ctx, localStackRef)
	if err != nil {
		return false, err
	}
	defer b.Unlock(ctx, localStackRef)

	snapshot, _, err := b.getStack(ctx, localStackRef)
	if err != nil {
		return false, err
	}

	// Don't remove stacks that still have resources.
	if !force && snapshot != nil && len(snapshot.Resources) > 0 {
		return true, errors.New("refusing to remove stack because it still contains resources")
	}

	return false, b.removeStack(localStackRef)
}

func (b *localBackend) RenameStack(ctx context.Context, stack backend.Stack,
	newName tokens.QName) (backend.StackReference, error) {
	localStackRef, err := b.getReference(stack.Ref())
	if err != nil {
		return nil, err
	}

	// Ensure the new stack name is valid.
	newRef, err := b.parseStackReference(string(newName))
	if err != nil {
		return nil, err
	}

	err = b.renameStack(ctx, localStackRef, newRef)
	if err != nil {
		return nil, err
	}

	return newRef, nil
}

func (b *localBackend) renameStack(ctx context.Context, oldRef *localBackendReference,
	newRef *localBackendReference) error {
	err := b.Lock(ctx, oldRef)
	if err != nil {
		return err
	}
	defer b.Unlock(ctx, oldRef)

	// Get the current state from the stack to be renamed.
	snap, _, err := b.getStack(ctx, oldRef)
	if err != nil {
		return err
	}

	// Ensure the destination stack does not already exist.
	hasExisting, err := b.bucket.Exists(ctx, b.stackPath(newRef))
	if err != nil {
		return err
	}
	if hasExisting {
		return fmt.Errorf("a stack named %s already exists", newRef.String())
	}

	// If we have a snapshot, we need to rename the URNs inside it to use the new stack name.
	if snap != nil {
		if err = edit.RenameStack(snap, newRef.name, ""); err != nil {
			return err
		}
	}

	// Now save the snapshot with a new name (we pass nil to re-use the existing secrets manager from the snapshot).
	if _, err = b.saveStack(newRef, snap, nil); err != nil {
		return err
	}

	// To remove the old stack, just make a backup of the file and don't write out anything new.
	file := b.stackPath(oldRef)
	backupTarget(b.bucket, file, false)

	// And rename the history folder as well.
	if err = b.renameHistory(oldRef, newRef); err != nil {
		return err
	}
	return err
}

func (b *localBackend) GetLatestConfiguration(ctx context.Context,
	stack backend.Stack) (config.Map, error) {

	hist, err := b.GetHistory(ctx, stack.Ref(), 1 /*pageSize*/, 1 /*page*/)
	if err != nil {
		return nil, err
	}
	if len(hist) == 0 {
		return nil, backend.ErrNoPreviousDeployment
	}

	return hist[0].Config, nil
}

func (b *localBackend) PackPolicies(
	ctx context.Context, policyPackRef backend.PolicyPackReference,
	cancellationScopes backend.CancellationScopeSource,
	callerEventsOpt chan<- engine.Event) result.Result {

	return result.Error("File state backend does not support resource policy")
}

func (b *localBackend) Preview(ctx context.Context, stack backend.Stack,
	op backend.UpdateOperation) (*deploy.Plan, sdkDisplay.ResourceChanges, result.Result) {
	// We can skip PreviewThenPromptThenExecute and just go straight to Execute.
	opts := backend.ApplierOptions{
		DryRun:   true,
		ShowLink: true,
	}
	return b.apply(ctx, apitype.PreviewUpdate, stack, op, opts, nil /*events*/)
}

func (b *localBackend) Update(ctx context.Context, stack backend.Stack,
	op backend.UpdateOperation) (sdkDisplay.ResourceChanges, result.Result) {

	err := b.Lock(ctx, stack.Ref())
	if err != nil {
		return nil, result.FromError(err)
	}
	defer b.Unlock(ctx, stack.Ref())

	return backend.PreviewThenPromptThenExecute(ctx, apitype.UpdateUpdate, stack, op, b.apply)
}

func (b *localBackend) Import(ctx context.Context, stack backend.Stack,
	op backend.UpdateOperation, imports []deploy.Import) (sdkDisplay.ResourceChanges, result.Result) {

	err := b.Lock(ctx, stack.Ref())
	if err != nil {
		return nil, result.FromError(err)
	}
	defer b.Unlock(ctx, stack.Ref())

	op.Imports = imports
	return backend.PreviewThenPromptThenExecute(ctx, apitype.ResourceImportUpdate, stack, op, b.apply)
}

func (b *localBackend) Refresh(ctx context.Context, stack backend.Stack,
	op backend.UpdateOperation) (sdkDisplay.ResourceChanges, result.Result) {

	err := b.Lock(ctx, stack.Ref())
	if err != nil {
		return nil, result.FromError(err)
	}
	defer b.Unlock(ctx, stack.Ref())

	return backend.PreviewThenPromptThenExecute(ctx, apitype.RefreshUpdate, stack, op, b.apply)
}

func (b *localBackend) Destroy(ctx context.Context, stack backend.Stack,
	op backend.UpdateOperation) (sdkDisplay.ResourceChanges, result.Result) {

	err := b.Lock(ctx, stack.Ref())
	if err != nil {
		return nil, result.FromError(err)
	}
	defer b.Unlock(ctx, stack.Ref())

	return backend.PreviewThenPromptThenExecute(ctx, apitype.DestroyUpdate, stack, op, b.apply)
}

func (b *localBackend) Query(ctx context.Context, op backend.QueryOperation) result.Result {

	return b.query(ctx, op, nil /*events*/)
}

func (b *localBackend) Watch(ctx context.Context, stk backend.Stack,
	op backend.UpdateOperation, paths []string) result.Result {
	return backend.Watch(ctx, stack.DefaultSecretsProvider, b, stk, op, b.apply, paths)
}

// apply actually performs the provided type of update on a locally hosted stack.
func (b *localBackend) apply(
	ctx context.Context, kind apitype.UpdateKind, stack backend.Stack,
	op backend.UpdateOperation, opts backend.ApplierOptions,
	events chan<- engine.Event) (*deploy.Plan, sdkDisplay.ResourceChanges, result.Result) {

	stackRef := stack.Ref()
	localStackRef, err := b.getReference(stackRef)
	if err != nil {
		return nil, nil, result.FromError(err)
	}

	if currentProjectContradictsWorkspace(localStackRef) {
		return nil, nil, result.Errorf("provided project name %q doesn't match Pulumi.yaml", localStackRef.project)
	}

	stackName := stackRef.FullyQualifiedName()
	actionLabel := backend.ActionLabel(kind, opts.DryRun)

	if !(op.Opts.Display.JSONDisplay || op.Opts.Display.Type == display.DisplayWatch) {
		// Print a banner so it's clear this is a local deployment.
		fmt.Printf(op.Opts.Display.Color.Colorize(
			colors.SpecHeadline+"%s (%s):"+colors.Reset+"\n"), actionLabel, stackRef)
	}

	// Start the update.
	update, err := b.newUpdate(ctx, localStackRef, op)
	if err != nil {
		return nil, nil, result.FromError(err)
	}

	// Spawn a display loop to show events on the CLI.
	displayEvents := make(chan engine.Event)
	displayDone := make(chan bool)
	go display.ShowEvents(
		strings.ToLower(actionLabel), kind, stackName.Name(), op.Proj.Name,
		displayEvents, displayDone, op.Opts.Display, opts.DryRun)

	// Create a separate event channel for engine events that we'll pipe to both listening streams.
	engineEvents := make(chan engine.Event)

	scope := op.Scopes.NewScope(engineEvents, opts.DryRun)
	eventsDone := make(chan bool)
	go func() {
		// Pull in all events from the engine and send them to the two listeners.
		for e := range engineEvents {
			displayEvents <- e

			// If the caller also wants to see the events, stream them there also.
			if events != nil {
				events <- e
			}
		}

		close(eventsDone)
	}()

	// Create the management machinery.
	persister := b.newSnapshotPersister(localStackRef, op.SecretsManager)
	manager := backend.NewSnapshotManager(persister, update.GetTarget().Snapshot)
	engineCtx := &engine.Context{
		Cancel:          scope.Context(),
		Events:          engineEvents,
		SnapshotManager: manager,
		BackendClient:   backend.NewBackendClient(b, op.SecretsProvider),
	}

	// Perform the update
	start := time.Now().Unix()
	var plan *deploy.Plan
	var changes sdkDisplay.ResourceChanges
	var updateRes result.Result
	switch kind {
	case apitype.PreviewUpdate:
		plan, changes, updateRes = engine.Update(update, engineCtx, op.Opts.Engine, true)
	case apitype.UpdateUpdate:
		_, changes, updateRes = engine.Update(update, engineCtx, op.Opts.Engine, opts.DryRun)
	case apitype.ResourceImportUpdate:
		_, changes, updateRes = engine.Import(update, engineCtx, op.Opts.Engine, op.Imports, opts.DryRun)
	case apitype.RefreshUpdate:
		_, changes, updateRes = engine.Refresh(update, engineCtx, op.Opts.Engine, opts.DryRun)
	case apitype.DestroyUpdate:
		_, changes, updateRes = engine.Destroy(update, engineCtx, op.Opts.Engine, opts.DryRun)
	default:
		contract.Failf("Unrecognized update kind: %s", kind)
	}
	end := time.Now().Unix()

	// Wait for the display to finish showing all the events.
	<-displayDone
	scope.Close() // Don't take any cancellations anymore, we're shutting down.
	close(engineEvents)
	contract.IgnoreClose(manager)

	// Make sure the goroutine writing to displayEvents and events has exited before proceeding.
	<-eventsDone
	close(displayEvents)

	// Save update results.
	backendUpdateResult := backend.SucceededResult
	if updateRes != nil {
		backendUpdateResult = backend.FailedResult
	}
	info := backend.UpdateInfo{
		Kind:        kind,
		StartTime:   start,
		Message:     op.M.Message,
		Environment: op.M.Environment,
		Config:      update.GetTarget().Config,
		Result:      backendUpdateResult,
		EndTime:     end,
		// IDEA: it would be nice to populate the *Deployment, so that addToHistory below doesn't need to
		//     rudely assume it knows where the checkpoint file is on disk as it makes a copy of it.  This isn't
		//     trivial to achieve today given the event driven nature of plan-walking, however.
		ResourceChanges: changes,
	}

	var saveErr error
	var backupErr error
	if !opts.DryRun {
		saveErr = b.addToHistory(localStackRef, info)
		backupErr = b.backupStack(localStackRef)
	}

	if updateRes != nil {
		// We swallow saveErr and backupErr as they are less important than the updateErr.
		return plan, changes, updateRes
	}

	if saveErr != nil {
		// We swallow backupErr as it is less important than the saveErr.
		return plan, changes, result.FromError(fmt.Errorf("saving update info: %w", saveErr))
	}

	if backupErr != nil {
		return plan, changes, result.FromError(fmt.Errorf("saving backup: %w", backupErr))
	}

	// Make sure to print a link to the stack's checkpoint before exiting.
	if !op.Opts.Display.SuppressPermalink && opts.ShowLink && !op.Opts.Display.JSONDisplay {
		// Note we get a real signed link for aws/azure/gcp links.  But no such option exists for
		// file:// links so we manually create the link ourselves.
		var link string
		if strings.HasPrefix(b.url, FilePathPrefix) {
			u, _ := url.Parse(b.url)
			u.Path = filepath.ToSlash(path.Join(u.Path, b.stackPath(localStackRef)))
			link = u.String()
		} else {
			link, err = b.bucket.SignedURL(context.TODO(), b.stackPath(localStackRef), nil)
			if err != nil {
				// set link to be empty to when there is an error to hide use of Permalinks
				link = ""

				// we log a warning here rather then returning an error to avoid exiting
				// pulumi with an error code.
				// printing a statefile perma link happens after all the providers have finished
				// deploying the infrastructure, failing the pulumi update because there was a
				// problem printing a statefile perma link can be missleading in automated CI environments.
				cmdutil.Diag().Warningf(diag.Message("", "Unable to create signed url for current backend to "+
					"create a Permalink. Please visit https://www.pulumi.com/docs/troubleshooting/ "+
					"for more information\n"))
			}
		}

		if link != "" {
			fmt.Printf(op.Opts.Display.Color.Colorize(
				colors.SpecHeadline+"Permalink: "+
					colors.Underline+colors.BrightBlue+"%s"+colors.Reset+"\n"), link)
		}
	}

	return plan, changes, nil
}

// query executes a query program against the resource outputs of a locally hosted stack.
func (b *localBackend) query(ctx context.Context, op backend.QueryOperation,
	callerEventsOpt chan<- engine.Event) result.Result {

	return backend.RunQuery(ctx, b, op, callerEventsOpt, b.newQuery)
}

func (b *localBackend) GetHistory(
	ctx context.Context,
	stackRef backend.StackReference,
	pageSize int,
	page int) ([]backend.UpdateInfo, error) {
	localStackRef, err := b.getReference(stackRef)
	if err != nil {
		return nil, err
	}
	updates, err := b.getHistory(localStackRef, pageSize, page)
	if err != nil {
		return nil, err
	}
	return updates, nil
}

func (b *localBackend) GetLogs(ctx context.Context,
	secretsProvider secrets.Provider, stack backend.Stack, cfg backend.StackConfiguration,
	query operations.LogQuery) ([]operations.LogEntry, error) {

	localStackRef, err := b.getReference(stack.Ref())
	if err != nil {
		return nil, err
	}

	target, err := b.getTarget(ctx, localStackRef, cfg.Config, cfg.Decrypter)
	if err != nil {
		return nil, err
	}

	return GetLogsForTarget(target, query)
}

// GetLogsForTarget fetches stack logs using the config, decrypter, and checkpoint in the given target.
func GetLogsForTarget(target *deploy.Target, query operations.LogQuery) ([]operations.LogEntry, error) {
	contract.Assert(target != nil)

	if target.Snapshot == nil {
		// If the stack has not been deployed yet, return no logs.
		return nil, nil
	}

	config, err := target.Config.Decrypt(target.Decrypter)
	if err != nil {
		return nil, err
	}

	components := operations.NewResourceTree(target.Snapshot.Resources)
	ops := components.OperationsProvider(config)
	logs, err := ops.GetLogs(query)
	if logs == nil {
		return nil, err
	}
	return *logs, err
}

func (b *localBackend) ExportDeployment(ctx context.Context,
	stk backend.Stack) (*apitype.UntypedDeployment, error) {

	localStackRef, err := b.getReference(stk.Ref())
	if err != nil {
		return nil, err
	}

	chk, err := b.getCheckpoint(localStackRef)
	if err != nil {
		return nil, fmt.Errorf("failed to load checkpoint: %w", err)
	}

	data, err := encoding.JSON.Marshal(chk.Latest)
	if err != nil {
		return nil, err
	}

	return &apitype.UntypedDeployment{
		Version:    3,
		Deployment: json.RawMessage(data),
	}, nil
}

func (b *localBackend) ImportDeployment(ctx context.Context, stk backend.Stack,
	deployment *apitype.UntypedDeployment) error {

	localStackRef, err := b.getReference(stk.Ref())
	if err != nil {
		return err
	}

	err = b.Lock(ctx, localStackRef)
	if err != nil {
		return err
	}
	defer b.Unlock(ctx, localStackRef)

	stackName := localStackRef.FullyQualifiedName()
	chk, err := stack.MarshalUntypedDeploymentToVersionedCheckpoint(stackName, deployment)
	if err != nil {
		return err
	}

	_, _, err = b.saveCheckpoint(localStackRef, chk)
	return err
}

func (b *localBackend) Logout() error {
	return workspace.DeleteAccount(b.originalURL)
}

func (b *localBackend) LogoutAll() error {
	return workspace.DeleteAllAccounts()
}

func (b *localBackend) CurrentUser() (string, []string, error) {
	user, err := user.Current()
	if err != nil {
		return "", nil, err
	}
	return user.Username, nil, nil
}

func (b *localBackend) getLocalStacks() ([]*localBackendReference, error) {
	// Read the stack directory.
	path := b.stackPath(nil)

	files, err := listBucket(b.bucket, path)
	if err != nil {
		return nil, fmt.Errorf("error listing stacks: %w", err)
	}
	var stacks = make([]*localBackendReference, 0, len(files))

	if b.projectMode {
		for _, file := range files {
			if file.IsDir {
				projName := objectName(file)
				// If this isn't a valid Name it won't be a project directory, so skip it
				if !tokens.IsName(projName) {
					continue
				}

				// TODO: Could we improve the efficiency here by firstly making listBucket return an enumerator not
				// eagerly collecting all keys into a slice, and secondly by getting listBucket to return all
				// descendent items not just the immediate children. We could then do the necessary splitting by
				// file paths here to work out project names.
				projectFiles, err := listBucket(b.bucket, filepath.Join(path, projName))
				if err != nil {
					return nil, fmt.Errorf("error listing stacks: %w", err)
				}

				for _, projectFile := range projectFiles {
					// Can ignore directories at this level
					if projectFile.IsDir {
						continue
					}

					objName := objectName(projectFile)
					// Skip files without valid extensions (e.g., *.bak files).
					ext := filepath.Ext(objName)
					// But accept gzip compression
					if ext == encoding.GZIPExt {
						objName = strings.TrimSuffix(objName, encoding.GZIPExt)
						ext = filepath.Ext(objName)
					}

					if _, has := encoding.Marshalers[ext]; !has {
						continue
					}

					// Read in this stack's information.
					name := objName[:len(objName)-len(ext)]
					stacks = append(stacks, &localBackendReference{
						project: tokens.Name(projName),
						name:    tokens.Name(name),
					})
				}
			}
		}
	} else {
		for _, file := range files {
			objName := objectName(file)
			// Skip files without valid extensions (e.g., *.bak files).
			ext := filepath.Ext(objName)
			// But accept gzip compression
			if ext == encoding.GZIPExt {
				objName = strings.TrimSuffix(objName, encoding.GZIPExt)
				ext = filepath.Ext(objName)
			}

			if _, has := encoding.Marshalers[ext]; !has {
				continue
			}

			// Read in this stack's information.
			name := objName[:len(objName)-len(ext)]
			stacks = append(stacks, &localBackendReference{
				name: tokens.Name(name),
			})
		}
	}

	return stacks, nil
}

func (b *localBackend) getLocalProjects() ([]tokens.Name, error) {
	// Read the stack directory.
	path := b.stackPath(nil)

	files, err := listBucket(b.bucket, path)
	if err != nil {
		return nil, fmt.Errorf("error listing projects: %w", err)
	}
	var projects = make([]tokens.Name, 0, len(files))

	for _, file := range files {
		// Ignore files.
		if !file.IsDir {
			continue
		}

		// Skip directories without valid names
		objName := objectName(file)
		if !tokens.IsName(objName) {
			continue
		}

		projects = append(projects, tokens.Name(objName))
	}

	return projects, nil
}

// GetStackTags fetches the stack's existing tags.
func (b *localBackend) GetStackTags(ctx context.Context,
	stack backend.Stack) (map[apitype.StackTagName]string, error) {

	// The local backend does not currently persist tags.
	return nil, errors.New("stack tags not supported in --local mode")
}

// UpdateStackTags updates the stacks's tags, replacing all existing tags.
func (b *localBackend) UpdateStackTags(ctx context.Context,
	stack backend.Stack, tags map[apitype.StackTagName]string) error {

	// The local backend does not currently persist tags.
	return errors.New("stack tags not supported in --local mode")
}

func (b *localBackend) CancelCurrentUpdate(ctx context.Context, stackRef backend.StackReference) error {
	// Try to delete ALL the lock files
	allFiles, err := listBucket(b.bucket, stackLockDir(stackRef.FullyQualifiedName()))
	if err != nil {
		// Don't error if it just wasn't found
		if gcerrors.Code(err) == gcerrors.NotFound {
			return nil
		}
		return err
	}

	for _, file := range allFiles {
		if file.IsDir {
			continue
		}

		err := b.bucket.Delete(ctx, file.Key)
		if err != nil {
			// Race condition, don't error if the file was delete between us calling list and now
			if gcerrors.Code(err) == gcerrors.NotFound {
				return nil
			}
			return err
		}
	}

	return nil
}
