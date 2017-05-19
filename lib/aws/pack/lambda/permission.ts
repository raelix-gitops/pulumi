// *** WARNING: this file was generated by the Lumi IDL Compiler (LUMIDL). ***
// *** Do not edit by hand unless you're certain you know what you are doing! ***

import * as lumi from "@lumi/lumi";

import {ARN} from "../types";
import {Function} from "./function";

export class Permission extends lumi.Resource implements PermissionArgs {
    public readonly name: string;
    public readonly action: string;
    public readonly function: Function;
    public readonly principal: string;
    public readonly sourceAccount?: string;
    public readonly sourceARN?: ARN;

    constructor(name: string, args: PermissionArgs) {
        super();
        if (name === undefined) {
            throw new Error("Missing required resource name");
        }
        this.name = name;
        if (args.action === undefined) {
            throw new Error("Missing required argument 'action'");
        }
        this.action = args.action;
        if (args.function === undefined) {
            throw new Error("Missing required argument 'function'");
        }
        this.function = args.function;
        if (args.principal === undefined) {
            throw new Error("Missing required argument 'principal'");
        }
        this.principal = args.principal;
        this.sourceAccount = args.sourceAccount;
        this.sourceARN = args.sourceARN;
    }
}

export interface PermissionArgs {
    readonly action: string;
    readonly function: Function;
    readonly principal: string;
    readonly sourceAccount?: string;
    readonly sourceARN?: ARN;
}


