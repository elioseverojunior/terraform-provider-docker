package provider

import (
	"context"
	"fmt"

	"github.com/elioseverojunior/terraform-provider-docker/internal/docker"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &PluginDataSource{}

type PluginDataSource struct {
	client *docker.Client
}

type PluginDataSourceModel struct {
	ID              types.String `tfsdk:"id"`
	Name            types.String `tfsdk:"name"`
	PluginReference types.String `tfsdk:"plugin_reference"`
	Enabled         types.Bool   `tfsdk:"enabled"`
	Env             types.List   `tfsdk:"env"`
}

func NewPluginDataSource() datasource.DataSource {
	return &PluginDataSource{}
}

func (d *PluginDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_plugin"
}

func (d *PluginDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Reads information about a Docker plugin.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The ID of this data source.",
				Computed:    true,
			},
			"name": schema.StringAttribute{
				Description: "The name or ID of the plugin.",
				Required:    true,
			},
			"plugin_reference": schema.StringAttribute{
				Description: "The plugin reference (image name).",
				Computed:    true,
			},
			"enabled": schema.BoolAttribute{
				Description: "Whether the plugin is enabled.",
				Computed:    true,
			},
			"env": schema.ListAttribute{
				Description: "Environment variables for the plugin.",
				Computed:    true,
				ElementType: types.StringType,
			},
		},
	}
}

func (d *PluginDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *PluginDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data PluginDataSourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	pluginName := data.Name.ValueString()

	plugin, _, err := d.client.PluginInspectWithRaw(ctx, pluginName)
	if err != nil {
		resp.Diagnostics.AddError(
			"Failed to Read Plugin",
			fmt.Sprintf("Unable to read plugin %s: %s", pluginName, err),
		)
		return
	}

	data.ID = types.StringValue(plugin.ID)
	data.PluginReference = types.StringValue(plugin.PluginReference)
	data.Enabled = types.BoolValue(plugin.Enabled)

	// Convert environment variables to list
	if plugin.Settings.Env != nil {
		envList, diags := types.ListValueFrom(ctx, types.StringType, plugin.Settings.Env)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		data.Env = envList
	} else {
		data.Env = types.ListNull(types.StringType)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
