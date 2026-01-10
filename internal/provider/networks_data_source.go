package provider

import (
	"context"
	"fmt"

	"github.com/docker/docker/api/types/network"
	"github.com/elioseverojunior/terraform-provider-docker/internal/docker"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &NetworksDataSource{}

type NetworksDataSource struct {
	client *docker.Client
}

type NetworksDataSourceModel struct {
	ID       types.String       `tfsdk:"id"`
	Networks []NetworkItemModel `tfsdk:"networks"`
}

type NetworkItemModel struct {
	ID       types.String `tfsdk:"id"`
	Name     types.String `tfsdk:"name"`
	Driver   types.String `tfsdk:"driver"`
	Scope    types.String `tfsdk:"scope"`
	Internal types.Bool   `tfsdk:"internal"`
	Labels   types.Map    `tfsdk:"labels"`
	Options  types.Map    `tfsdk:"options"`
}

func NewNetworksDataSource() datasource.DataSource {
	return &NetworksDataSource{}
}

func (d *NetworksDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_networks"
}

func (d *NetworksDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Use this data source to list all Docker networks.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The ID of this data source.",
				Computed:    true,
			},
			"networks": schema.ListNestedAttribute{
				Description: "List of Docker networks.",
				Computed:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							Description: "The network ID.",
							Computed:    true,
						},
						"name": schema.StringAttribute{
							Description: "The network name.",
							Computed:    true,
						},
						"driver": schema.StringAttribute{
							Description: "The network driver (e.g., bridge, overlay).",
							Computed:    true,
						},
						"scope": schema.StringAttribute{
							Description: "The network scope (local or swarm).",
							Computed:    true,
						},
						"internal": schema.BoolAttribute{
							Description: "Whether the network is internal.",
							Computed:    true,
						},
						"labels": schema.MapAttribute{
							Description: "User-defined key/value metadata.",
							Computed:    true,
							ElementType: types.StringType,
						},
						"options": schema.MapAttribute{
							Description: "Driver-specific options.",
							Computed:    true,
							ElementType: types.StringType,
						},
					},
				},
			},
		},
	}
}

func (d *NetworksDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	providerData, ok := req.ProviderData.(*ProviderData)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected *ProviderData, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}

	d.client = providerData.DockerClient
}

func (d *NetworksDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data NetworksDataSourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// List all networks
	networks, err := d.client.NetworkList(ctx, network.ListOptions{})
	if err != nil {
		resp.Diagnostics.AddError("Failed to List Networks", fmt.Sprintf("Unable to list Docker networks: %s", err))
		return
	}

	data.ID = types.StringValue("docker_networks")
	data.Networks = make([]NetworkItemModel, len(networks))

	for i, net := range networks {
		item := NetworkItemModel{
			ID:       types.StringValue(net.ID),
			Name:     types.StringValue(net.Name),
			Driver:   types.StringValue(net.Driver),
			Scope:    types.StringValue(net.Scope),
			Internal: types.BoolValue(net.Internal),
		}

		// Convert labels map
		if len(net.Labels) > 0 {
			labels, diags := types.MapValueFrom(ctx, types.StringType, net.Labels)
			resp.Diagnostics.Append(diags...)
			item.Labels = labels
		} else {
			item.Labels = types.MapNull(types.StringType)
		}

		// Convert options map
		if len(net.Options) > 0 {
			options, diags := types.MapValueFrom(ctx, types.StringType, net.Options)
			resp.Diagnostics.Append(diags...)
			item.Options = options
		} else {
			item.Options = types.MapNull(types.StringType)
		}

		data.Networks[i] = item
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
