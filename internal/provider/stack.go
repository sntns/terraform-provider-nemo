package provider

import (
	"context"
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/sntns/nemo/pkg/target"
)

var _ resource.Resource = (*stackResource)(nil)

// dockerName is what Docker accepts in the names nemo gives after the stack.
var dockerName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]*$`)

type stackResource struct{}

func newStackResource() resource.Resource {
	return &stackResource{}
}

func (r *stackResource) Metadata(_ context.Context, request resource.MetadataRequest, response *resource.MetadataResponse) {
	response.TypeName = request.ProviderTypeName + "_stack"
}

func (r *stackResource) Schema(_ context.Context, _ resource.SchemaRequest, response *resource.SchemaResponse) {
	response.Schema = schema.Schema{
		MarkdownDescription: "A stack: bundles deployed together on one target, as nemo deploys a manifest with `nemo up`. " +
			"Each apply brings the stack to its configuration: it installs the bundles it lists, upgrades those installed, " +
			"and uninstalls those it no longer lists. Reading it reads what the target runs, so that an installation changed by hand " +
			"shows in the next plan. Destroying it uninstalls every installation.\n\n" +
			"The records of its installations are kept on its target (`records: target` in nemo's manifest): " +
			"nemo reads them from any machine that imported `manifest`, and refuses to write the stack without `--force`.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "The stack's name.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "The stack's name: Swarm names its services `<name>_<service>`, in 63 characters. Changing it replaces the stack.",
				Required:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
				Validators:          []validator.String{stringvalidator.RegexMatches(dockerName, "must be letters, digits, _ . and - only")},
			},
			"target":    targetSchema(),
			"installer": installerSchema(),
			"remote": schema.StringAttribute{
				MarkdownDescription: "Where the bundles of the installations that give a version alone are: a template of " +
					"`{{ .Component }}`, the installation's name, and `{{ .Version }}`, its version, such as " +
					"`docker://ghcr.io/sntns/{{ .Component }}:{{ .Version }}`.",
				Optional: true,
			},
			"parameters": schema.MapAttribute{
				MarkdownDescription: "Parameters, by name, for every installation whose bundle declares them. An installation's own come over them.",
				ElementType:         types.StringType,
				Optional:            true,
			},
			"credentials": credentialsSchema("Credentials, by name, for every installation whose bundle declares them. An installation's own come over them."),
			"installations": schema.ListNestedAttribute{
				MarkdownDescription: "The installations of the stack, one per bundle, installed in the order of their dependencies.",
				Required:            true,
				NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
					"name": schema.StringAttribute{
						MarkdownDescription: "The installation's name, its bundle's.",
						Required:            true,
					},
					"version": schema.StringAttribute{
						MarkdownDescription: "The bundle's version: with `remote`, or with `repository`. Reading the stack sets the version deployed.",
						Optional:            true,
					},
					"repository": schema.StringAttribute{
						MarkdownDescription: "The bundle's repository in a registry, instead of `remote`: `ghcr.io/sntns/whoami`.",
						Optional:            true,
					},
					"file": schema.StringAttribute{
						MarkdownDescription: "A `bundle.json`, instead of `remote` or `repository`.",
						Optional:            true,
					},
					"parameters": schema.MapAttribute{
						MarkdownDescription: "This installation's parameters, over the stack's.",
						ElementType:         types.StringType,
						Optional:            true,
					},
					"credentials": credentialsSchema("This installation's credentials, over the stack's."),
				}},
			},
			"purge_on_destroy": schema.BoolAttribute{
				MarkdownDescription: "Destroying the stack also removes its volumes, **data included**. Default: `false`, the volumes are kept for the next install.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
			},
			"manifest": schema.StringAttribute{
				MarkdownDescription: "The stack's manifest, in YAML, for `nemo stack import`: nemo then reads the stack, " +
					"its status and its logs, from where it reaches the target. Its credentials given by `value` are in it.",
				Computed:  true,
				Sensitive: true,
			},
		},
	}
}

func targetSchema() schema.Attribute {
	options := schema.MapAttribute{
		MarkdownDescription: "The driver's options.",
		ElementType:         types.StringType,
		Optional:            true,
	}
	return schema.SingleNestedAttribute{
		MarkdownDescription: "Where the stack is deployed: one Docker, and the orchestrator that deploys there.",
		Required:            true,
		Attributes: map[string]schema.Attribute{
			"orchestrator": schema.StringAttribute{
				MarkdownDescription: "`swarm` or `compose`. Changing it replaces the stack.",
				Required:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
				Validators:          []validator.String{stringvalidator.OneOf(target.Swarm, target.Compose)},
			},
			"host": schema.StringAttribute{
				MarkdownDescription: "The address of the target's Docker: `tcp://`, `ssh://user@host` or `unix://`. " +
					"With neither `host` nor `context`, the environment's: `DOCKER_HOST`, the current docker context, or the local socket.",
				Optional:   true,
				Validators: []validator.String{stringvalidator.ConflictsWith(path.MatchRelative().AtParent().AtName("context"))},
			},
			"context": schema.StringAttribute{
				MarkdownDescription: "A docker context to reach the target with, instead of `host`.",
				Optional:            true,
			},
			"tls": schema.SingleNestedAttribute{
				MarkdownDescription: "The TLS of a `tcp://` host, in PEM: the CA that signed the daemon's certificate, and the client's certificate and key. " +
					"They are not in `manifest`: nemo reaches the target as its user's environment says.",
				Optional: true,
				Attributes: map[string]schema.Attribute{
					"ca":   schema.StringAttribute{MarkdownDescription: "The CA's certificate.", Required: true},
					"cert": schema.StringAttribute{MarkdownDescription: "The client's certificate.", Required: true},
					"key":  schema.StringAttribute{MarkdownDescription: "The client's key.", Required: true, Sensitive: true},
				},
			},
			"images": schema.StringAttribute{
				MarkdownDescription: "How the images reach the target: `registry`, which each node pulls them from, or `load`, into its one daemon. Default: `registry`.",
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString(target.ImagesRegistry),
				Validators:          []validator.String{stringvalidator.OneOf(target.ImagesRegistry, target.ImagesLoad)},
			},
			"volumes": schema.MapNestedAttribute{
				MarkdownDescription: "The target's volume types, by name: what a volume of a bundle with that `x-nemo-volume` type is made of. " +
					"The driver, its options and the provisioned directory are templates of `{{.Stack}}`, `{{.Name}}` and `{{.Options}}`.",
				Optional: true,
				NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
					"driver":  schema.StringAttribute{MarkdownDescription: "The volume driver. Default: `local`.", Optional: true},
					"options": options,
					"provision": schema.SingleNestedAttribute{
						MarkdownDescription: "A directory to create for each volume before the volume, in the storage `volume` mounts.",
						Optional:            true,
						Attributes: map[string]schema.Attribute{
							"directory": schema.StringAttribute{MarkdownDescription: "The directory, in the storage.", Required: true},
							"volume": schema.SingleNestedAttribute{
								MarkdownDescription: "The storage the directory is created in.",
								Required:            true,
								Attributes: map[string]schema.Attribute{
									"driver":  schema.StringAttribute{MarkdownDescription: "The volume driver. Default: `local`.", Optional: true},
									"options": options,
								},
							},
						},
					},
				}},
			},
		},
	}
}

func installerSchema() schema.Attribute {
	return schema.SingleNestedAttribute{
		MarkdownDescription: "Where gill, the installer, runs: on the target, the default, or as a Job in a Kubernetes cluster that reaches the target over `tcp://`.",
		Optional:            true,
		Attributes: map[string]schema.Attribute{
			"driver": schema.StringAttribute{
				MarkdownDescription: "`docker` or `kubernetes`. Default: `docker`.",
				Optional:            true,
				Validators:          []validator.String{stringvalidator.OneOf(target.DriverDocker, target.DriverKubernetes)},
			},
			"kubernetes": schema.SingleNestedAttribute{
				MarkdownDescription: "The cluster of the `kubernetes` driver.",
				Optional:            true,
				Attributes: map[string]schema.Attribute{
					"namespace":  schema.StringAttribute{MarkdownDescription: "The namespace of the Job.", Optional: true},
					"kubeconfig": schema.StringAttribute{MarkdownDescription: "The kubeconfig of the cluster. Default: `KUBECONFIG`, `~/.kube/config`, else the cluster Terraform runs in.", Optional: true},
				},
			},
		},
	}
}

// credentialsSchema is a map of credential sources, each exactly one of
// a file, a variable, a command or a value, read where Terraform runs.
func credentialsSchema(description string) schema.Attribute {
	return schema.MapNestedAttribute{
		MarkdownDescription: description + " Each is read where Terraform runs, from one source.",
		Optional:            true,
		NestedObject: schema.NestedAttributeObject{
			Attributes: map[string]schema.Attribute{
				"path":    schema.StringAttribute{MarkdownDescription: "A file.", Optional: true},
				"env":     schema.StringAttribute{MarkdownDescription: "An environment variable.", Optional: true},
				"command": schema.StringAttribute{MarkdownDescription: "A command, whose output is the value.", Optional: true},
				"value":   schema.StringAttribute{MarkdownDescription: "The value itself.", Optional: true, Sensitive: true},
			},
			Validators: []validator.Object{oneSource{}},
		},
	}
}

// The resource's actions come once its schema is agreed on.

func (r *stackResource) Create(_ context.Context, _ resource.CreateRequest, response *resource.CreateResponse) {
	response.Diagnostics.AddError("Not implemented yet", "This version of the nemo provider has the schema of nemo_stack only.")
}

func (r *stackResource) Read(_ context.Context, _ resource.ReadRequest, response *resource.ReadResponse) {
	response.Diagnostics.AddError("Not implemented yet", "This version of the nemo provider has the schema of nemo_stack only.")
}

func (r *stackResource) Update(_ context.Context, _ resource.UpdateRequest, response *resource.UpdateResponse) {
	response.Diagnostics.AddError("Not implemented yet", "This version of the nemo provider has the schema of nemo_stack only.")
}

func (r *stackResource) Delete(_ context.Context, _ resource.DeleteRequest, response *resource.DeleteResponse) {
	response.Diagnostics.AddError("Not implemented yet", "This version of the nemo provider has the schema of nemo_stack only.")
}
