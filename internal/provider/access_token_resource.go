package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/elioseverojunior/terraform-provider-docker/internal/dockerhub"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	tftypes "github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

var (
	_ resource.Resource                = &AccessTokenResource{}
	_ resource.ResourceWithImportState = &AccessTokenResource{}
)

type AccessTokenResource struct {
	hubClient *dockerhub.Client
}

type AccessTokenResourceModel struct {
	ID       tftypes.String `tfsdk:"id"`
	UUID     tftypes.String `tfsdk:"uuid"`
	Label    tftypes.String `tfsdk:"label"`
	Scopes   tftypes.List   `tfsdk:"scopes"`
	IsActive tftypes.Bool   `tfsdk:"is_active"`
	Token    tftypes.String `tfsdk:"token"`
}

func NewAccessTokenResource() resource.Resource {
	return &AccessTokenResource{}
}

func (r *AccessTokenResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_access_token"
}

func (r *AccessTokenResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages Docker Hub Personal Access Tokens (PATs).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The ID of this resource (same as uuid).",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"uuid": schema.StringAttribute{
				Description: "The UUID of the access token.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"label": schema.StringAttribute{
				Description: "A label/description for the access token.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"scopes": schema.ListAttribute{
				Description: "List of scopes/permissions for the token. Valid values: repo:read, repo:write, repo:admin, repo:public_read.",
				Optional:    true,
				Computed:    true,
				ElementType: tftypes.StringType,
				PlanModifiers: []planmodifier.List{
					listplanmodifier.RequiresReplace(),
				},
			},
			"is_active": schema.BoolAttribute{
				Description: "Whether the token is active. Default: true.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(true),
			},
			"token": schema.StringAttribute{
				Description: "The access token value. Only available at creation time.",
				Computed:    true,
				Sensitive:   true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *AccessTokenResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

	r.hubClient = providerData.HubClient
}

func (r *AccessTokenResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data AccessTokenResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if r.hubClient == nil {
		resp.Diagnostics.AddError(
			"Docker Hub Not Configured",
			"Docker Hub credentials are required to manage access tokens. Configure hub_username and hub_password in the provider.",
		)
		return
	}

	var scopes []string
	if !data.Scopes.IsNull() && !data.Scopes.IsUnknown() {
		resp.Diagnostics.Append(data.Scopes.ElementsAs(ctx, &scopes, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	token := &dockerhub.AccessToken{
		Label:    data.Label.ValueString(),
		Scopes:   scopes,
		IsActive: data.IsActive.ValueBool(),
	}

	tflog.Debug(ctx, "Creating Docker Hub access token", map[string]interface{}{
		"label": token.Label,
	})

	result, err := r.hubClient.CreateAccessToken(ctx, token)
	if err != nil {
		resp.Diagnostics.AddError(
			"Docker Hub Access Token Creation Failed",
			fmt.Sprintf("Failed to create access token %s: %s", token.Label, err),
		)
		return
	}

	data.ID = tftypes.StringValue(result.UUID)
	data.UUID = tftypes.StringValue(result.UUID)
	data.Token = tftypes.StringValue(result.Token)

	if len(result.Scopes) > 0 {
		scopesList, diags := tftypes.ListValueFrom(ctx, tftypes.StringType, result.Scopes)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		data.Scopes = scopesList
	}

	tflog.Debug(ctx, "Created Docker Hub access token", map[string]interface{}{
		"uuid": result.UUID,
	})

	resp.Diagnostics.AddWarning(
		"Access Token Created",
		"The access token has been created. The token value is only available now and cannot be retrieved later. Store it securely.",
	)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *AccessTokenResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data AccessTokenResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if r.hubClient == nil {
		resp.Diagnostics.AddError(
			"Docker Hub Not Configured",
			"Docker Hub credentials are required to read access tokens.",
		)
		return
	}

	uuid := data.UUID.ValueString()

	result, err := r.hubClient.GetAccessToken(ctx, uuid)
	if err != nil {
		if strings.Contains(err.Error(), "404") || strings.Contains(err.Error(), "not found") {
			tflog.Debug(ctx, "Access token not found, removing from state", map[string]interface{}{
				"uuid": uuid,
			})
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Docker Hub Access Token Read Failed",
			fmt.Sprintf("Failed to read access token %s: %s", uuid, err),
		)
		return
	}

	data.Label = tftypes.StringValue(result.Label)
	data.IsActive = tftypes.BoolValue(result.IsActive)

	if len(result.Scopes) > 0 {
		scopesList, diags := tftypes.ListValueFrom(ctx, tftypes.StringType, result.Scopes)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		data.Scopes = scopesList
	}

	// Token value is never returned after creation
	// Keep whatever is in state

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *AccessTokenResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data AccessTokenResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if r.hubClient == nil {
		resp.Diagnostics.AddError(
			"Docker Hub Not Configured",
			"Docker Hub credentials are required to update access tokens.",
		)
		return
	}

	// Only is_active can be updated
	uuid := data.UUID.ValueString()
	isActive := data.IsActive.ValueBool()

	tflog.Debug(ctx, "Updating Docker Hub access token", map[string]interface{}{
		"uuid":      uuid,
		"is_active": isActive,
	})

	_, err := r.hubClient.UpdateAccessToken(ctx, uuid, isActive)
	if err != nil {
		resp.Diagnostics.AddError(
			"Docker Hub Access Token Update Failed",
			fmt.Sprintf("Failed to update access token %s: %s", uuid, err),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *AccessTokenResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data AccessTokenResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if r.hubClient == nil {
		resp.Diagnostics.AddError(
			"Docker Hub Not Configured",
			"Docker Hub credentials are required to delete access tokens.",
		)
		return
	}

	uuid := data.UUID.ValueString()

	tflog.Debug(ctx, "Deleting Docker Hub access token", map[string]interface{}{
		"uuid": uuid,
	})

	err := r.hubClient.DeleteAccessToken(ctx, uuid)
	if err != nil {
		if strings.Contains(err.Error(), "404") || strings.Contains(err.Error(), "not found") {
			tflog.Debug(ctx, "Access token already deleted", map[string]interface{}{
				"uuid": uuid,
			})
			return
		}
		resp.Diagnostics.AddError(
			"Docker Hub Access Token Deletion Failed",
			fmt.Sprintf("Failed to delete access token %s: %s", uuid, err),
		)
		return
	}

	tflog.Debug(ctx, "Deleted Docker Hub access token", map[string]interface{}{
		"uuid": uuid,
	})
}

func (r *AccessTokenResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Import by UUID
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("uuid"), req.ID)...)
	resp.Diagnostics.AddWarning(
		"Token Value Not Available",
		"The access token value cannot be retrieved after creation. You will need to regenerate the token if you need the value.",
	)
}
