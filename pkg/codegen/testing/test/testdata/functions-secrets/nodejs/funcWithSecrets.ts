// *** WARNING: this file was generated by test. ***
// *** Do not edit by hand unless you're certain you know what you are doing! ***

import * as pulumi from "@pulumi/pulumi";
import * as utilities from "./utilities";

export function funcWithSecrets(args: FuncWithSecretsArgs, opts?: pulumi.InvokeOptions): Promise<FuncWithSecretsResult> {
    if (!opts) {
        opts = {}
    }

    opts = pulumi.mergeOptions(utilities.resourceOptsDefaults(), opts);
    return pulumi.runtime.invoke("mypkg::funcWithSecrets", {
        "cryptoKey": args.cryptoKey,
        "plaintext": args.plaintext,
    }, opts);
}

export interface FuncWithSecretsArgs {
    cryptoKey: string;
    plaintext: string;
}

export interface FuncWithSecretsResult {
    readonly ciphertext: string;
    readonly cryptoKey: string;
    readonly id: string;
    readonly plaintext: string;
}

export function funcWithSecretsOutput(args: FuncWithSecretsOutputArgs, opts?: pulumi.InvokeOptions): pulumi.Output<FuncWithSecretsResult> {
    return pulumi.output(args).apply(a => funcWithSecrets(a, opts))
}

export interface FuncWithSecretsOutputArgs {
    cryptoKey: pulumi.Input<string>;
    plaintext: pulumi.Input<string>;
}
