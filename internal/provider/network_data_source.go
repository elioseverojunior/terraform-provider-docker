package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/docker/docker/api/types/network"
	"github.com/elioseverojunior/terraform-provider-docker/internal/docker"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &NetworkDataSource{}

type NetworkDataSource struct {
	client *docker.Client
}

type NetworkDataSourceModel struct {
	ID       types.String `tfsdk:"id"`
	Name     types.String `tfsdk:"name"`
	Driver   types.String `tfsdk:"driver"`
	Scope    types.String `tfsdk:"scope"`
	Internal types.Bool   `tfsdk:"internal"`
	Labels   types.Map    `tfsdk:"labels"`
	Options  types.Map    `tfsdk:"options"`
}

func NewNetworkDataSource() datasource.DataSource {
	return &NetworkDataSource{}
}

func (d *NetworkDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_network"
}

func (d *NetworkDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Use this data source to get information about a Docker network.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The ID of this resource.",
				Computed:    true,
			},
			"name": schema.StringAttribute{
				Description: "The name of the Docker network.",
				Required:    true,
			},
			"driver": schema.StringAttribute{
				Description: "The driver of the Docker network.",
				Computed:    true,
			},
			"scope": schema.StringAttribute{
				Description: "The scope of the network.",
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
	}
}

func (d *NetworkDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *NetworkDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data NetworkDataSourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	networkName := data.Name.ValueString()

	networkInspect, err := d.client.NetworkInspect(ctx, networkName, network.InspectOptions{})
	if err != nil {
		if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "No such network") {
			resp.Diagnostics.AddError("Network Not Found", fmt.Sprintf("Network %s not found", networkName))
			return
		}
		resp.Diagnostics.AddError("Network Read Error", fmt.Sprintf("Unable to read network %s: %s", networkName, err))
		return
	}

	data.ID = types.StringValue(networkInspect.ID)
	data.Name = types.StringValue(networkInspect.Name)
	data.Driver = types.StringValue(networkInspect.Driver)
	data.Scope = types.StringValue(networkInspect.Scope)
	data.Internal = types.BoolValue(networkInspect.Internal)

	// Labels
	if len(networkInspect.Labels) > 0 {
		labels, diags := types.MapValueFrom(ctx, types.StringType, networkInspect.Labels)
		resp.Diagnostics.Append(diags...)
		data.Labels = labels
	} else {
		data.Labels = types.MapNull(types.StringType)
	}

	// Options
	if len(networkInspect.Options) > 0 {
		options, diags := types.MapValueFrom(ctx, types.StringType, networkInspect.Options)
		resp.Diagnostics.Append(diags...)
		data.Options = options
	} else {
		data.Options = types.MapNull(types.StringType)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
