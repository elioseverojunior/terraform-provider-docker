package provider

import (
	"context"
	"fmt"

	"github.com/elioseverojunior/terraform-provider-docker/internal/dockerhub"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &OrgMembersDataSource{}

type OrgMembersDataSource struct {
	hubClient *dockerhub.Client
}

type OrgMembersDataSourceModel struct {
	ID      types.String       `tfsdk:"id"`
	OrgName types.String       `tfsdk:"org_name"`
	Members []OrgMemberModel   `tfsdk:"members"`
}

type OrgMemberModel struct {
	ID       types.String `tfsdk:"id"`
	Username types.String `tfsdk:"username"`
	FullName types.String `tfsdk:"full_name"`
	Email    types.String `tfsdk:"email"`
	Role     types.String `tfsdk:"role"`
}

func NewOrgMembersDataSource() datasource.DataSource {
	return &OrgMembersDataSource{}
}

func (d *OrgMembersDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_org_members"
}

func (d *OrgMembersDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Lists members of a Docker Hub organization.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The ID of this data source.",
				Computed:    true,
			},
			"org_name": schema.StringAttribute{
				Description: "The organization name.",
				Required:    true,
			},
			"members": schema.ListNestedAttribute{
				Description: "List of organization members.",
				Computed:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							Description: "The member ID.",
							Computed:    true,
						},
						"username": schema.StringAttribute{
							Description: "The member's username.",
							Computed:    true,
						},
						"full_name": schema.StringAttribute{
							Description: "The member's full name.",
							Computed:    true,
						},
						"email": schema.StringAttribute{
							Description: "The member's email.",
							Computed:    true,
						},
						"role": schema.StringAttribute{
							Description: "The member's role (member or owner).",
							Computed:    true,
						},
					},
				},
			},
		},
	}
}

func (d *OrgMembersDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *OrgMembersDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data OrgMembersDataSourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if d.hubClient == nil {
		resp.Diagnostics.AddError(
			"Docker Hub Not Configured",
			"Docker Hub credentials are required to list organization members.",
		)
		return
	}

	orgName := data.OrgName.ValueString()

	// Fetch all members (paginated)
	var allMembers []dockerhub.Member
	page := 1
	pageSize := 100

	for {
		members, count, err := d.hubClient.ListOrgMembers(ctx, orgName, page, pageSize)
		if err != nil {
			resp.Diagnostics.AddError(
				"Failed to List Organization Members",
				fmt.Sprintf("Unable to list members for %s: %s", orgName, err),
			)
			return
		}

		allMembers = append(allMembers, members...)

		if len(allMembers) >= count || len(members) == 0 {
			break
		}
		page++
	}

	data.ID = types.StringValue(orgName)
	data.Members = make([]OrgMemberModel, len(allMembers))

	for i, member := range allMembers {
		data.Members[i] = OrgMemberModel{
			ID:       types.StringValue(member.ID),
			Username: types.StringValue(member.Username),
			FullName: types.StringValue(member.FullName),
			Email:    types.StringValue(member.Email),
			Role:     types.StringValue(member.Role),
		}
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
