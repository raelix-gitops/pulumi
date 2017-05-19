// *** WARNING: this file was generated by the Lumi IDL Compiler (LUMIDL). ***
// *** Do not edit by hand unless you're certain you know what you are doing! ***

import * as lumi from "@lumi/lumi";

import {VPC} from "./vpc";

export class VPCPeeringConnection extends lumi.Resource implements VPCPeeringConnectionArgs {
    public readonly name: string;
    public readonly peerVpc: VPC;
    public readonly vpc: VPC;

    constructor(name: string, args: VPCPeeringConnectionArgs) {
        super();
        if (name === undefined) {
            throw new Error("Missing required resource name");
        }
        this.name = name;
        if (args.peerVpc === undefined) {
            throw new Error("Missing required argument 'peerVpc'");
        }
        this.peerVpc = args.peerVpc;
        if (args.vpc === undefined) {
            throw new Error("Missing required argument 'vpc'");
        }
        this.vpc = args.vpc;
    }
}

export interface VPCPeeringConnectionArgs {
    readonly peerVpc: VPC;
    readonly vpc: VPC;
}


