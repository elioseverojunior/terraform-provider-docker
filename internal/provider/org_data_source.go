package provider

import (
	"context"
	"fmt"

	"github.com/elioseverojunior/terraform-provider-docker/internal/dockerhub"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &OrgDataSource{}

type OrgDataSource struct {
	hubClient *dockerhub.Client
}

type OrgDataSourceModel struct {
	ID         types.String `tfsdk:"id"`
	OrgName    types.String `tfsdk:"org_name"`
	FullName   types.String `tfsdk:"full_name"`
	Location   types.String `tfsdk:"location"`
	Company    types.String `tfsdk:"company"`
	DateJoined types.String `tfsdk:"date_joined"`
}

func NewOrgDataSource() datasource.DataSource {
	return &OrgDataSource{}
}

func (d *OrgDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_org"
}

func (d *OrgDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Reads information about a Docker Hub organization.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The ID of this data source.",
				Computed:    true,
			},
			"org_name": schema.StringAttribute{
				Description: "The organization name.",
				Required:    true,
			},
			"full_name": schema.StringAttribute{
				Description: "The full name of the organization.",
				Computed:    true,
			},
			"location": schema.StringAttribute{
				Description: "The location of the organization.",
				Computed:    true,
			},
			"company": schema.StringAttribute{
				Description: "The company name.",
				Computed:    true,
			},
			"date_joined": schema.StringAttribute{
				Description: "When the organization was created.",
				Computed:    true,
			},
		},
	}
}

func (d *OrgDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

	d.hubClient = providerData.HubClient
}

func (d *OrgDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data OrgDataSourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if d.hubClient == nil {
		resp.Diagnostics.AddError(
			"Docker Hub Not Configured",
			"Docker Hub credentials are required to read organization information.",
		)
		return
	}

	orgName := data.OrgName.ValueString()

	result, err := d.hubClient.GetOrganization(ctx, orgName)
	if err != nil {
		resp.Diagnostics.AddError(
			"Failed to Read Organization",
			fmt.Sprintf("Unable to read organization %s: %s", orgName, err),
		)
		return
	}

	data.ID = types.StringValue(result.ID)
	data.FullName = types.StringValue(result.FullName)
	data.Location = types.StringValue(result.Location)
	data.Company = types.StringValue(result.Company)
	if !result.DateJoined.IsZero() {
		data.DateJoined = types.StringValue(result.DateJoined.Format("2006-01-02T15:04:05Z"))
	} else {
		data.DateJoined = types.StringValue("")
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
