// *** WARNING: this file was generated by the Lumi IDL Compiler (LUMIDL). ***
// *** Do not edit by hand unless you're certain you know what you are doing! ***

import * as lumi from "@lumi/lumi";

import {RestAPI} from "./restAPI";

export class Model extends lumi.Resource implements ModelArgs {
    public readonly name: string;
    public readonly contentType: string;
    public readonly restAPI: RestAPI;
    public schema: any;
    public readonly modelName?: string;
    public description?: string;

    constructor(name: string, args: ModelArgs) {
        super();
        if (name === undefined) {
            throw new Error("Missing required resource name");
        }
        this.name = name;
        if (args.contentType === undefined) {
            throw new Error("Missing required argument 'contentType'");
        }
        this.contentType = args.contentType;
        if (args.restAPI === undefined) {
            throw new Error("Missing required argument 'restAPI'");
        }
        this.restAPI = args.restAPI;
        if (args.schema === undefined) {
            throw new Error("Missing required argument 'schema'");
        }
        this.schema = args.schema;
        this.modelName = args.modelName;
        this.description = args.description;
    }
}

export interface ModelArgs {
    readonly contentType: string;
    readonly restAPI: RestAPI;
    schema: any;
    readonly modelName?: string;
    description?: string;
}


