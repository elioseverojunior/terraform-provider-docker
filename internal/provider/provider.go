package provider

import (
	"context"
	"os"

	"github.com/elioseverojunior/terraform-provider-docker/internal/docker"
	"github.com/elioseverojunior/terraform-provider-docker/internal/dockerhub"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

var _ provider.Provider = &DockerProvider{}

type DockerProvider struct {
	version string
}

// ProviderData holds both Docker Engine and Hub clients
type ProviderData struct {
	DockerClient *docker.Client
	HubClient    *dockerhub.Client
}

type DockerProviderModel struct {
	// Docker Engine configuration
	Host      types.String `tfsdk:"host"`
	TLSVerify types.Bool   `tfsdk:"tls_verify"`
	CertPath  types.String `tfsdk:"cert_path"`
	CACert    types.String `tfsdk:"ca_cert"`
	Cert      types.String `tfsdk:"cert"`
	Key       types.String `tfsdk:"key"`

	// Docker Hub configuration
	HubUsername types.String `tfsdk:"hub_username"`
	HubPassword types.String `tfsdk:"hub_password"`
	HubToken    types.String `tfsdk:"hub_token"`
}

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &DockerProvider{
			version: version,
		}
	}
}

func (p *DockerProvider) Metadata(ctx context.Context, req provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "docker"
	resp.Version = p.version
}

func (p *DockerProvider) Schema(ctx context.Context, req provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "The Docker provider is used to interact with Docker containers, images, networks, volumes, Swarm services, and Docker Hub resources.",
		Attributes: map[string]schema.Attribute{
			// Docker Engine attributes
			"host": schema.StringAttribute{
				Description: "The Docker daemon socket to connect to. Defaults to unix:///var/run/docker.sock. Can also be set via DOCKER_HOST environment variable.",
				Optional:    true,
			},
			"tls_verify": schema.BoolAttribute{
				Description: "Enable TLS verification for remote Docker hosts. Can also be set via DOCKER_TLS_VERIFY environment variable.",
				Optional:    true,
			},
			"cert_path": schema.StringAttribute{
				Description: "Path to directory containing TLS certificates (ca.pem, cert.pem, key.pem). Can also be set via DOCKER_CERT_PATH environment variable.",
				Optional:    true,
			},
			"ca_cert": schema.StringAttribute{
				Description: "PEM-encoded CA certificate content for TLS verification.",
				Optional:    true,
				Sensitive:   true,
			},
			"cert": schema.StringAttribute{
				Description: "PEM-encoded client certificate content for TLS authentication.",
				Optional:    true,
				Sensitive:   true,
			},
			"key": schema.StringAttribute{
				Description: "PEM-encoded client key content for TLS authentication.",
				Optional:    true,
				Sensitive:   true,
			},

			// Docker Hub attributes
			"hub_username": schema.StringAttribute{
				Description: "Docker Hub username. Can also be set via DOCKER_HUB_USERNAME environment variable. Required for Docker Hub resources.",
				Optional:    true,
			},
			"hub_password": schema.StringAttribute{
				Description: "Docker Hub password. Can also be set via DOCKER_HUB_PASSWORD environment variable. Use with hub_username for full Docker Hub access.",
				Optional:    true,
				Sensitive:   true,
			},
			"hub_token": schema.StringAttribute{
				Description: "Docker Hub Personal Access Token (PAT). Can also be set via DOCKER_HUB_TOKEN environment variable. Alternative to hub_password for repository-only access.",
				Optional:    true,
				Sensitive:   true,
			},
		},
	}
}

func (p *DockerProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var config DockerProviderModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Docker Engine configuration
	host := os.Getenv("DOCKER_HOST")
	if !config.Host.IsNull() {
		host = config.Host.ValueString()
	}
	if host == "" {
		host = "unix:///var/run/docker.sock"
	}

	tlsVerify := os.Getenv("DOCKER_TLS_VERIFY") == "1"
	if !config.TLSVerify.IsNull() {
		tlsVerify = config.TLSVerify.ValueBool()
	}

	certPath := os.Getenv("DOCKER_CERT_PATH")
	if !config.CertPath.IsNull() {
		certPath = config.CertPath.ValueString()
	}

	var caCert, cert, key string
	if !config.CACert.IsNull() {
		caCert = config.CACert.ValueString()
	}
	if !config.Cert.IsNull() {
		cert = config.Cert.ValueString()
	}
	if !config.Key.IsNull() {
		key = config.Key.ValueString()
	}

	// Validate TLS configuration
	if tlsVerify {
		hasCertPath := certPath != ""
		hasCertContent := caCert != "" && cert != "" && key != ""

		if !hasCertPath && !hasCertContent {
			resp.Diagnostics.AddAttributeError(
				path.Root("tls_verify"),
				"Missing TLS Configuration",
				"When tls_verify is enabled, you must provide either cert_path or all of ca_cert, cert, and key.",
			)
			return
		}
	}

	// Create Docker Engine client
	clientConfig := docker.ClientConfig{
		Host:      host,
		TLSVerify: tlsVerify,
		CertPath:  certPath,
		CACert:    caCert,
		Cert:      cert,
		Key:       key,
	}

	dockerClient, err := docker.NewClient(clientConfig)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Create Docker Client",
			"An unexpected error occurred when creating the Docker client. "+
				"If the error is not clear, please contact the provider developers.\n\n"+
				"Docker Client Error: "+err.Error(),
		)
		return
	}

	// Docker Hub configuration
	hubUsername := os.Getenv("DOCKER_HUB_USERNAME")
	if !config.HubUsername.IsNull() {
		hubUsername = config.HubUsername.ValueString()
	}

	hubPassword := os.Getenv("DOCKER_HUB_PASSWORD")
	if !config.HubPassword.IsNull() {
		hubPassword = config.HubPassword.ValueString()
	}

	hubToken := os.Getenv("DOCKER_HUB_TOKEN")
	if !config.HubToken.IsNull() {
		hubToken = config.HubToken.ValueString()
	}

	// Create Docker Hub client if credentials are provided
	var hubClient *dockerhub.Client
	if hubUsername != "" && (hubPassword != "" || hubToken != "") {
		hubConfig := dockerhub.Config{
			Username: hubUsername,
			Password: hubPassword,
			Token:    hubToken,
		}

		hubClient, err = dockerhub.NewClient(ctx, hubConfig)
		if err != nil {
			// Don't fail provider initialization, just warn
			tflog.Warn(ctx, "Failed to create Docker Hub client", map[string]interface{}{
				"error": err.Error(),
			})
			resp.Diagnostics.AddWarning(
				"Docker Hub Authentication Failed",
				"Failed to authenticate with Docker Hub: "+err.Error()+
					"\nDocker Hub resources will not be available.",
			)
		} else {
			tflog.Info(ctx, "Docker Hub client initialized successfully")
		}
	}

	// Create provider data containing both clients
	providerData := &ProviderData{
		DockerClient: dockerClient,
		HubClient:    hubClient,
	}

	resp.DataSourceData = providerData
	resp.ResourceData = providerData
}

func (p *DockerProvider) Resources(ctx context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		// Docker Engine resources
		NewImageResource,
		NewNetworkResource,
		NewVolumeResource,
		NewContainerResource,
		NewComposeResource,

		// Swarm resources
		NewSecretResource,
		NewConfigResource,
		NewServiceResource,

		// Registry resources
		NewTagResource,
		NewRegistryImageResource,

		// Docker Hub resources (require hub credentials)
		NewHubRepositoryResource,
		NewHubRepositoryTeamPermissionResource,
		NewOrgTeamResource,
		NewOrgMemberResource,
		NewOrgTeamMemberResource,
		NewAccessTokenResource,
	}
}

func (p *DockerProvider) DataSources(ctx context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		// Docker Engine data sources
		NewImageDataSource,
		NewNetworkDataSource,
		NewContainerDataSource,
		NewComposeDataSource,
		NewLogsDataSource,
		NewPluginDataSource,
		NewRegistryImageDataSource,

		// Docker Hub data sources
		NewHubRepositoryDataSource,
		NewHubRepositoriesDataSource,
		NewHubRepositoryTagsDataSource,
		NewOrgDataSource,
		NewOrgMembersDataSource,
		NewOrgTeamDataSource,
		NewAccessTokensDataSource,
	}
}
