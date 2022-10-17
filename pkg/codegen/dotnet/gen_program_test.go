package dotnet

import (
	"testing"

	"github.com/pulumi/pulumi/pkg/v3/codegen"
	"github.com/pulumi/pulumi/pkg/v3/codegen/pcl"
	"github.com/pulumi/pulumi/pkg/v3/codegen/testing/test"
	"github.com/pulumi/pulumi/sdk/v3/go/common/workspace"
)

func TestGenerateProgramVersionSelection(t *testing.T) {
	t.Parallel()

	expectedVersion := map[string]test.PkgVersionInfo{
		"aws-resource-options-4.26": {
			Pkg:          "<PackageReference Include=\"Pulumi.Aws\"",
			OpAndVersion: "Version=\"4.26.0\"",
		},
		"aws-resource-options-5.16.2": {
			Pkg:          "<PackageReference Include=\"Pulumi.Aws\"",
			OpAndVersion: "Version=\"5.16.2\"",
		},
	}

	test.TestProgramCodegen(t,
		test.ProgramCodegenOptions{
			Language:   "dotnet",
			Extension:  "cs",
			OutputFile: "Program.cs",
			Check: func(t *testing.T, path string, dependencies codegen.StringSet) {
				Check(t, path, dependencies, "../../../../../../../sdk/dotnet/Pulumi")
			},
			GenProgram: GenerateProgram,
			TestCases: []test.ProgramTest{
				{
					Directory:   "aws-resource-options-4.26",
					Description: "Resource Options",
				},
				{
					Directory:   "aws-resource-options-5.16.2",
					Description: "Resource Options",
				},
			},

			IsGenProject: true,
			GenProject: func(directory string, project workspace.Project, program *pcl.Program) error {
				return GenerateProject(directory, project, program, false)
			},
			ExpectedVersion: expectedVersion,
			DependencyFile:  "test.csproj",
		},
	)
}
