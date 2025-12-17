package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/docker/docker/api/types/network"
	"github.com/elioseverojunior/terraform-provider-docker/internal/docker"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

var (
	_ resource.Resource                = &NetworkResource{}
	_ resource.ResourceWithImportState = &NetworkResource{}
)

type NetworkResource struct {
	client *docker.Client
}

type NetworkResourceModel struct {
	ID         types.String      `tfsdk:"id"`
	Name       types.String      `tfsdk:"name"`
	Driver     types.String      `tfsdk:"driver"`
	Internal   types.Bool        `tfsdk:"internal"`
	Attachable types.Bool        `tfsdk:"attachable"`
	Ingress    types.Bool        `tfsdk:"ingress"`
	Labels     types.Map         `tfsdk:"labels"`
	Options    types.Map         `tfsdk:"options"`
	Scope      types.String      `tfsdk:"scope"`
	IPAM       *NetworkIPAMModel `tfsdk:"ipam"`
	IPv6       types.Bool        `tfsdk:"ipv6"`
}

type NetworkIPAMModel struct {
	Driver  types.String        `tfsdk:"driver"`
	Config  []NetworkIPAMConfig `tfsdk:"config"`
	Options types.Map           `tfsdk:"options"`
}

type NetworkIPAMConfig struct {
	Subnet     types.String `tfsdk:"subnet"`
	IPRange    types.String `tfsdk:"ip_range"`
	Gateway    types.String `tfsdk:"gateway"`
	AuxAddress types.Map    `tfsdk:"aux_address"`
}

func NewNetworkResource() resource.Resource {
	return &NetworkResource{}
}

func (r *NetworkResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_network"
}

func (r *NetworkResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages Docker networks.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The ID of this resource.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Description: "The name of the Docker network.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"driver": schema.StringAttribute{
				Description: "The driver of the Docker network. Possible values are bridge, host, overlay, macvlan. Default is bridge.",
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString("bridge"),
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"internal": schema.BoolAttribute{
				Description: "Whether the network is internal (restricts external access). Default is false.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
			},
			"attachable": schema.BoolAttribute{
				Description: "Whether the network is attachable. Default is false.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
			},
			"ingress": schema.BoolAttribute{
				Description: "Whether the network is an ingress network (Swarm). Default is false.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
			},
			"ipv6": schema.BoolAttribute{
				Description: "Enable IPv6 networking. Default is false.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
			},
			"labels": schema.MapAttribute{
				Description: "User-defined key/value metadata.",
				Optional:    true,
				ElementType: types.StringType,
			},
			"options": schema.MapAttribute{
				Description: "Driver-specific options.",
				Optional:    true,
				ElementType: types.StringType,
			},
			"scope": schema.StringAttribute{
				Description: "The scope of the network (local, swarm, global).",
				Computed:    true,
			},
		},
		Blocks: map[string]schema.Block{
			"ipam": schema.SingleNestedBlock{
				Description: "IPAM (IP Address Management) configuration.",
				Attributes: map[string]schema.Attribute{
					"driver": schema.StringAttribute{
						Description: "IPAM driver. Default is 'default'.",
						Optional:    true,
						Computed:    true,
						Default:     stringdefault.StaticString("default"),
					},
					"options": schema.MapAttribute{
						Description: "IPAM driver options.",
						Optional:    true,
						ElementType: types.StringType,
					},
				},
				Blocks: map[string]schema.Block{
					"config": schema.ListNestedBlock{
						Description: "IPAM configuration blocks.",
						NestedObject: schema.NestedBlockObject{
							Attributes: map[string]schema.Attribute{
								"subnet": schema.StringAttribute{
									Description: "The subnet in CIDR form.",
									Optional:    true,
								},
								"ip_range": schema.StringAttribute{
									Description: "The IP range within the subnet.",
									Optional:    true,
								},
								"gateway": schema.StringAttribute{
									Description: "The gateway for the subnet.",
									Optional:    true,
								},
								"aux_address": schema.MapAttribute{
									Description: "Auxiliary addresses for the network.",
									Optional:    true,
									ElementType: types.StringType,
								},
							},
						},
					},
				},
			},
		},
	}
}

func (r *NetworkResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	providerData, ok := req.ProviderData.(*ProviderData)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *ProviderData, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}

	r.client = providerData.DockerClient
}

func (r *NetworkResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data NetworkResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	networkName := data.Name.ValueString()
	tflog.Debug(ctx, "Creating Docker network", map[string]interface{}{
		"name": networkName,
	})

	// Build network create options
	createOptions := network.CreateOptions{
		Driver:     data.Driver.ValueString(),
		Internal:   data.Internal.ValueBool(),
		Attachable: data.Attachable.ValueBool(),
		Ingress:    data.Ingress.ValueBool(),
		EnableIPv6: &[]bool{data.IPv6.ValueBool()}[0],
	}

	// Labels
	if !data.Labels.IsNull() {
		labels := make(map[string]string)
		resp.Diagnostics.Append(data.Labels.ElementsAs(ctx, &labels, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
		createOptions.Labels = labels
	}

	// Driver options
	if !data.Options.IsNull() {
		options := make(map[string]string)
		resp.Diagnostics.Append(data.Options.ElementsAs(ctx, &options, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
		createOptions.Options = options
	}

	// IPAM configuration
	if data.IPAM != nil {
		ipamConfig := &network.IPAM{
			Driver: data.IPAM.Driver.ValueString(),
		}

		if !data.IPAM.Options.IsNull() {
			options := make(map[string]string)
			resp.Diagnostics.Append(data.IPAM.Options.ElementsAs(ctx, &options, false)...)
			if resp.Diagnostics.HasError() {
				return
			}
			ipamConfig.Options = options
		}

		for _, cfg := range data.IPAM.Config {
			ipamCfg := network.IPAMConfig{
				Subnet:  cfg.Subnet.ValueString(),
				IPRange: cfg.IPRange.ValueString(),
				Gateway: cfg.Gateway.ValueString(),
			}

			if !cfg.AuxAddress.IsNull() {
				auxAddr := make(map[string]string)
				resp.Diagnostics.Append(cfg.AuxAddress.ElementsAs(ctx, &auxAddr, false)...)
				if resp.Diagnostics.HasError() {
					return
				}
				ipamCfg.AuxAddress = auxAddr
			}

			ipamConfig.Config = append(ipamConfig.Config, ipamCfg)
		}

		createOptions.IPAM = ipamConfig
	}

	// Create the network
	networkResp, err := r.client.NetworkCreate(ctx, networkName, createOptions)
	if err != nil {
		resp.Diagnostics.AddError("Network Create Error", fmt.Sprintf("Unable to create network %s: %s", networkName, err))
		return
	}

	data.ID = types.StringValue(networkResp.ID)

	// Inspect to get computed attributes
	networkInspect, err := r.client.NetworkInspect(ctx, networkResp.ID, network.InspectOptions{})
	if err != nil {
		resp.Diagnostics.AddError("Network Inspect Error", fmt.Sprintf("Unable to inspect network %s: %s", networkName, err))
		return
	}

	data.Scope = types.StringValue(networkInspect.Scope)

	tflog.Debug(ctx, "Created Docker network", map[string]interface{}{
		"name": networkName,
		"id":   networkResp.ID,
	})

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *NetworkResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data NetworkResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	networkID := data.ID.ValueString()

	networkInspect, err := r.client.NetworkInspect(ctx, networkID, network.InspectOptions{})
	if err != nil {
		if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "No such network") {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Network Read Error", fmt.Sprintf("Unable to read network %s: %s", networkID, err))
		return
	}

	data.Name = types.StringValue(networkInspect.Name)
	data.Driver = types.StringValue(networkInspect.Driver)
	data.Internal = types.BoolValue(networkInspect.Internal)
	data.Attachable = types.BoolValue(networkInspect.Attachable)
	data.Ingress = types.BoolValue(networkInspect.Ingress)
	data.Scope = types.StringValue(networkInspect.Scope)

	if networkInspect.EnableIPv6 {
		data.IPv6 = types.BoolValue(true)
	}

	// Labels
	if len(networkInspect.Labels) > 0 {
		labels, diags := types.MapValueFrom(ctx, types.StringType, networkInspect.Labels)
		resp.Diagnostics.Append(diags...)
		data.Labels = labels
	}

	// Options
	if len(networkInspect.Options) > 0 {
		options, diags := types.MapValueFrom(ctx, types.StringType, networkInspect.Options)
		resp.Diagnostics.Append(diags...)
		data.Options = options
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *NetworkResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data NetworkResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Docker networks are mostly immutable, so changes require recreation
	// Only labels can be updated, but the Docker API doesn't support this directly
	// For now, we just save the state as-is

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *NetworkResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data NetworkResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	networkID := data.ID.ValueString()

	tflog.Debug(ctx, "Deleting Docker network", map[string]interface{}{
		"id": networkID,
	})

	err := r.client.NetworkRemove(ctx, networkID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "No such network") {
			return
		}
		resp.Diagnostics.AddError("Network Delete Error", fmt.Sprintf("Unable to delete network %s: %s", networkID, err))
		return
	}

	tflog.Debug(ctx, "Deleted Docker network", map[string]interface{}{
		"id": networkID,
	})
}

func (r *NetworkResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
