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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	tftypes "github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

var (
	_ resource.Resource                = &HubRepositoryResource{}
	_ resource.ResourceWithImportState = &HubRepositoryResource{}
)

type HubRepositoryResource struct {
	hubClient *dockerhub.Client
}

type HubRepositoryResourceModel struct {
	ID              tftypes.String `tfsdk:"id"`
	Namespace       tftypes.String `tfsdk:"namespace"`
	Name            tftypes.String `tfsdk:"name"`
	Description     tftypes.String `tfsdk:"description"`
	FullDescription tftypes.String `tfsdk:"full_description"`
	Private         tftypes.Bool   `tfsdk:"private"`
	PullCount       tftypes.Int64  `tfsdk:"pull_count"`
	StarCount       tftypes.Int64  `tfsdk:"star_count"`
}

func NewHubRepositoryResource() resource.Resource {
	return &HubRepositoryResource{}
}

func (r *HubRepositoryResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_hub_repository"
}

func (r *HubRepositoryResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages Docker Hub repositories.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The ID of this resource (namespace/name).",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"namespace": schema.StringAttribute{
				Description: "The Docker Hub namespace (username or organization name).",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				Description: "The repository name.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"description": schema.StringAttribute{
				Description: "Short description of the repository (max 100 characters).",
				Optional:    true,
			},
			"full_description": schema.StringAttribute{
				Description: "Full description of the repository (markdown supported).",
				Optional:    true,
			},
			"private": schema.BoolAttribute{
				Description: "Whether the repository is private. Default is false.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
			},
			"pull_count": schema.Int64Attribute{
				Description: "Number of pulls for this repository.",
				Computed:    true,
			},
			"star_count": schema.Int64Attribute{
				Description: "Number of stars for this repository.",
				Computed:    true,
			},
		},
	}
}

func (r *HubRepositoryResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *HubRepositoryResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data HubRepositoryResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if r.hubClient == nil {
		resp.Diagnostics.AddError(
			"Docker Hub Not Configured",
			"Docker Hub credentials are required to manage Hub repositories. Configure hub_username and hub_password or hub_token in the provider.",
		)
		return
	}

	repo := &dockerhub.Repository{
		Namespace:       data.Namespace.ValueString(),
		Name:            data.Name.ValueString(),
		Description:     data.Description.ValueString(),
		FullDescription: data.FullDescription.ValueString(),
		IsPrivate:       data.Private.ValueBool(),
	}

	tflog.Debug(ctx, "Creating Docker Hub repository", map[string]interface{}{
		"namespace": repo.Namespace,
		"name":      repo.Name,
	})

	result, err := r.hubClient.CreateRepository(ctx, repo)
	if err != nil {
		resp.Diagnostics.AddError(
			"Docker Hub Repository Creation Failed",
			fmt.Sprintf("Failed to create repository %s/%s: %s", repo.Namespace, repo.Name, err),
		)
		return
	}

	data.ID = tftypes.StringValue(fmt.Sprintf("%s/%s", result.Namespace, result.Name))
	data.PullCount = tftypes.Int64Value(result.PullCount)
	data.StarCount = tftypes.Int64Value(result.StarCount)

	tflog.Debug(ctx, "Created Docker Hub repository", map[string]interface{}{
		"id": data.ID.ValueString(),
	})

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *HubRepositoryResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data HubRepositoryResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if r.hubClient == nil {
		resp.Diagnostics.AddError(
			"Docker Hub Not Configured",
			"Docker Hub credentials are required to read Hub repositories.",
		)
		return
	}

	namespace := data.Namespace.ValueString()
	name := data.Name.ValueString()

	result, err := r.hubClient.GetRepository(ctx, namespace, name)
	if err != nil {
		if strings.Contains(err.Error(), "404") || strings.Contains(err.Error(), "not found") {
			tflog.Debug(ctx, "Repository not found, removing from state", map[string]interface{}{
				"namespace": namespace,
				"name":      name,
			})
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(
			"Docker Hub Repository Read Failed",
			fmt.Sprintf("Failed to read repository %s/%s: %s", namespace, name, err),
		)
		return
	}

	data.Description = tftypes.StringValue(result.Description)
	data.FullDescription = tftypes.StringValue(result.FullDescription)
	data.Private = tftypes.BoolValue(result.IsPrivate)
	data.PullCount = tftypes.Int64Value(result.PullCount)
	data.StarCount = tftypes.Int64Value(result.StarCount)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *HubRepositoryResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data HubRepositoryResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if r.hubClient == nil {
		resp.Diagnostics.AddError(
			"Docker Hub Not Configured",
			"Docker Hub credentials are required to update Hub repositories.",
		)
		return
	}

	repo := &dockerhub.Repository{
		Namespace:       data.Namespace.ValueString(),
		Name:            data.Name.ValueString(),
		Description:     data.Description.ValueString(),
		FullDescription: data.FullDescription.ValueString(),
	}

	tflog.Debug(ctx, "Updating Docker Hub repository", map[string]interface{}{
		"namespace": repo.Namespace,
		"name":      repo.Name,
	})

	result, err := r.hubClient.UpdateRepository(ctx, repo)
	if err != nil {
		resp.Diagnostics.AddError(
			"Docker Hub Repository Update Failed",
			fmt.Sprintf("Failed to update repository %s/%s: %s", repo.Namespace, repo.Name, err),
		)
		return
	}

	data.PullCount = tftypes.Int64Value(result.PullCount)
	data.StarCount = tftypes.Int64Value(result.StarCount)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *HubRepositoryResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data HubRepositoryResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if r.hubClient == nil {
		resp.Diagnostics.AddError(
			"Docker Hub Not Configured",
			"Docker Hub credentials are required to delete Hub repositories.",
		)
		return
	}

	namespace := data.Namespace.ValueString()
	name := data.Name.ValueString()

	tflog.Debug(ctx, "Deleting Docker Hub repository", map[string]interface{}{
		"namespace": namespace,
		"name":      name,
	})

	err := r.hubClient.DeleteRepository(ctx, namespace, name)
	if err != nil {
		if strings.Contains(err.Error(), "404") || strings.Contains(err.Error(), "not found") {
			tflog.Debug(ctx, "Repository already deleted", map[string]interface{}{
				"namespace": namespace,
				"name":      name,
			})
			return
		}
		resp.Diagnostics.AddError(
			"Docker Hub Repository Deletion Failed",
			fmt.Sprintf("Failed to delete repository %s/%s: %s", namespace, name, err),
		)
		return
	}

	tflog.Debug(ctx, "Deleted Docker Hub repository", map[string]interface{}{
		"namespace": namespace,
		"name":      name,
	})
}

func (r *HubRepositoryResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Import by namespace/name
	parts := strings.Split(req.ID, "/")
	if len(parts) != 2 {
		resp.Diagnostics.AddError(
			"Invalid Import ID",
			fmt.Sprintf("Expected format: namespace/name, got: %s", req.ID),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("namespace"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), parts[1])...)
}
