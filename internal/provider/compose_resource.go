package provider

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	composetypes "github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/strslice"
	"github.com/docker/docker/api/types/volume"
	"github.com/docker/go-connections/nat"
	"github.com/elioseverojunior/terraform-provider-docker/internal/docker"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"gopkg.in/yaml.v3"
)

var (
	_ resource.Resource                = &ComposeResource{}
	_ resource.ResourceWithImportState = &ComposeResource{}
)

type ComposeResource struct {
	client *docker.Client
}

type ComposeResourceModel struct {
	ID              types.String `tfsdk:"id"`
	ProjectName     types.String `tfsdk:"project_name"`
	ComposeFile     types.String `tfsdk:"compose_file"`
	ComposeContent  types.String `tfsdk:"compose_content"`
	RemoveOrphans   types.Bool   `tfsdk:"remove_orphans"`
	RemoveVolumes   types.Bool   `tfsdk:"remove_volumes"`
	ForceRecreate   types.Bool   `tfsdk:"force_recreate"`
	ContentHash     types.String `tfsdk:"content_hash"`
	Services        types.List   `tfsdk:"services"`
	RunningServices types.Int64  `tfsdk:"running_services"`
}

func NewComposeResource() resource.Resource {
	return &ComposeResource{}
}

func (r *ComposeResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_compose"
}

func (r *ComposeResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages Docker Compose stacks using the Docker API directly (no CLI required).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The ID of this resource (project name).",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"project_name": schema.StringAttribute{
				Description: "The name of the Docker Compose project. Used to identify and label resources.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"compose_file": schema.StringAttribute{
				Description: "Path to the Docker Compose file. Either compose_file or compose_content must be specified.",
				Optional:    true,
			},
			"compose_content": schema.StringAttribute{
				Description: "Inline Docker Compose YAML content. Either compose_file or compose_content must be specified.",
				Optional:    true,
			},
			"remove_orphans": schema.BoolAttribute{
				Description: "Remove containers for services not defined in the Compose file. Default is true.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(true),
			},
			"remove_volumes": schema.BoolAttribute{
				Description: "Remove named volumes declared in the Compose file on destroy. Default is false.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
			},
			"force_recreate": schema.BoolAttribute{
				Description: "Recreate containers even if their configuration hasn't changed. Default is false.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(false),
			},
			"content_hash": schema.StringAttribute{
				Description: "Hash of the compose file content for change detection.",
				Computed:    true,
			},
			"services": schema.ListAttribute{
				Description: "List of service names in the compose stack.",
				Computed:    true,
				ElementType: types.StringType,
			},
			"running_services": schema.Int64Attribute{
				Description: "Number of running services in the stack.",
				Computed:    true,
			},
		},
	}
}

func (r *ComposeResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *ComposeResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data ComposeResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if data.ComposeFile.IsNull() && data.ComposeContent.IsNull() {
		resp.Diagnostics.AddError(
			"Missing Compose Configuration",
			"Either compose_file or compose_content must be specified.",
		)
		return
	}

	projectName := data.ProjectName.ValueString()
	tflog.Debug(ctx, "Creating Docker Compose stack", map[string]interface{}{
		"project_name": projectName,
	})

	// Parse compose file
	project, err := r.parseComposeFile(data)
	if err != nil {
		resp.Diagnostics.AddError("Compose Parse Error", fmt.Sprintf("Failed to parse compose file: %s", err))
		return
	}

	// Calculate content hash
	contentHash := r.calculateContentHash(data)
	data.ContentHash = types.StringValue(contentHash)

	// Create networks
	for name, netConfig := range project.Networks {
		if err := r.createNetwork(ctx, projectName, name, netConfig); err != nil {
			resp.Diagnostics.AddError("Network Create Error", fmt.Sprintf("Failed to create network %s: %s", name, err))
			return
		}
	}

	// Create volumes
	for name, volConfig := range project.Volumes {
		if err := r.createVolume(ctx, projectName, name, volConfig); err != nil {
			resp.Diagnostics.AddError("Volume Create Error", fmt.Sprintf("Failed to create volume %s: %s", name, err))
			return
		}
	}

	// Create and start containers in dependency order
	serviceOrder := r.getServiceOrder(project)
	for _, serviceName := range serviceOrder {
		service := project.Services[serviceName]
		if err := r.createService(ctx, projectName, serviceName, service, project); err != nil {
			resp.Diagnostics.AddError("Service Create Error", fmt.Sprintf("Failed to create service %s: %s", serviceName, err))
			return
		}
	}

	data.ID = types.StringValue(projectName)

	// Refresh stack info
	r.refreshStackInfo(ctx, &data, project)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *ComposeResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data ComposeResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Parse compose file to get service list
	project, err := r.parseComposeFile(data)
	if err != nil {
		// File might have been deleted, but stack might still exist
		tflog.Warn(ctx, "Failed to parse compose file", map[string]interface{}{
			"error": err.Error(),
		})
	}

	// Update content hash
	contentHash := r.calculateContentHash(data)
	data.ContentHash = types.StringValue(contentHash)

	// Refresh stack info
	r.refreshStackInfo(ctx, &data, project)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *ComposeResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data ComposeResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	projectName := data.ProjectName.ValueString()
	tflog.Debug(ctx, "Updating Docker Compose stack", map[string]interface{}{
		"project_name": projectName,
	})

	// Parse new compose file
	project, err := r.parseComposeFile(data)
	if err != nil {
		resp.Diagnostics.AddError("Compose Parse Error", fmt.Sprintf("Failed to parse compose file: %s", err))
		return
	}

	// Calculate new content hash
	contentHash := r.calculateContentHash(data)
	data.ContentHash = types.StringValue(contentHash)

	// Update networks
	for name, netConfig := range project.Networks {
		if err := r.createNetwork(ctx, projectName, name, netConfig); err != nil {
			tflog.Warn(ctx, "Network might already exist", map[string]interface{}{
				"network": name,
				"error":   err.Error(),
			})
		}
	}

	// Update volumes
	for name, volConfig := range project.Volumes {
		if err := r.createVolume(ctx, projectName, name, volConfig); err != nil {
			tflog.Warn(ctx, "Volume might already exist", map[string]interface{}{
				"volume": name,
				"error":  err.Error(),
			})
		}
	}

	// Recreate containers if force_recreate or config changed
	if data.ForceRecreate.ValueBool() {
		// Stop and remove existing containers
		r.removeProjectContainers(ctx, projectName)

		// Recreate containers
		serviceOrder := r.getServiceOrder(project)
		for _, serviceName := range serviceOrder {
			service := project.Services[serviceName]
			if err := r.createService(ctx, projectName, serviceName, service, project); err != nil {
				resp.Diagnostics.AddError("Service Create Error", fmt.Sprintf("Failed to recreate service %s: %s", serviceName, err))
				return
			}
		}
	}

	// Remove orphan containers if enabled
	if data.RemoveOrphans.ValueBool() {
		r.removeOrphanContainers(ctx, projectName, project)
	}

	// Refresh stack info
	r.refreshStackInfo(ctx, &data, project)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *ComposeResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data ComposeResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	projectName := data.ProjectName.ValueString()
	tflog.Debug(ctx, "Destroying Docker Compose stack", map[string]interface{}{
		"project_name": projectName,
	})

	// Stop and remove all containers for this project
	r.removeProjectContainers(ctx, projectName)

	// Remove networks
	r.removeProjectNetworks(ctx, projectName)

	// Remove volumes if configured
	if data.RemoveVolumes.ValueBool() {
		r.removeProjectVolumes(ctx, projectName)
	}
}

func (r *ComposeResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("project_name"), req, resp)
}

// parseComposeFile parses the compose file and returns a Project
func (r *ComposeResource) parseComposeFile(data ComposeResourceModel) (*composetypes.Project, error) {
	var content []byte
	var err error

	if !data.ComposeFile.IsNull() {
		content, err = os.ReadFile(data.ComposeFile.ValueString())
		if err != nil {
			return nil, fmt.Errorf("failed to read compose file: %w", err)
		}
	} else if !data.ComposeContent.IsNull() {
		content = []byte(data.ComposeContent.ValueString())
	} else {
		return nil, fmt.Errorf("no compose file or content specified")
	}

	// Parse YAML into a generic map first
	var rawConfig map[string]interface{}
	if err := yaml.Unmarshal(content, &rawConfig); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	// Convert to compose Project structure
	project := &composetypes.Project{
		Name:     data.ProjectName.ValueString(),
		Services: make(composetypes.Services),
		Networks: make(composetypes.Networks),
		Volumes:  make(composetypes.Volumes),
	}

	// Parse services
	if services, ok := rawConfig["services"].(map[string]interface{}); ok {
		for name, svcConfig := range services {
			service, err := parseService(name, svcConfig)
			if err != nil {
				return nil, fmt.Errorf("failed to parse service %s: %w", name, err)
			}
			project.Services[name] = service
		}
	}

	// Parse networks
	if networks, ok := rawConfig["networks"].(map[string]interface{}); ok {
		for name, netConfig := range networks {
			network := parseNetwork(netConfig)
			project.Networks[name] = network
		}
	}

	// Parse volumes
	if volumes, ok := rawConfig["volumes"].(map[string]interface{}); ok {
		for name, volConfig := range volumes {
			vol := parseVolume(volConfig)
			project.Volumes[name] = vol
		}
	}

	return project, nil
}

func parseService(name string, config interface{}) (composetypes.ServiceConfig, error) {
	svc := composetypes.ServiceConfig{
		Name: name,
	}

	if config == nil {
		return svc, nil
	}

	cfg, ok := config.(map[string]interface{})
	if !ok {
		return svc, nil
	}

	if image, ok := cfg["image"].(string); ok {
		svc.Image = image
	}

	if command, ok := cfg["command"]; ok {
		switch cmd := command.(type) {
		case string:
			svc.Command = composetypes.ShellCommand{cmd}
		case []interface{}:
			for _, c := range cmd {
				if s, ok := c.(string); ok {
					svc.Command = append(svc.Command, s)
				}
			}
		}
	}

	if entrypoint, ok := cfg["entrypoint"]; ok {
		switch ep := entrypoint.(type) {
		case string:
			svc.Entrypoint = composetypes.ShellCommand{ep}
		case []interface{}:
			for _, e := range ep {
				if s, ok := e.(string); ok {
					svc.Entrypoint = append(svc.Entrypoint, s)
				}
			}
		}
	}

	if env, ok := cfg["environment"]; ok {
		svc.Environment = make(composetypes.MappingWithEquals)
		switch e := env.(type) {
		case map[string]interface{}:
			for k, v := range e {
				if s, ok := v.(string); ok {
					svc.Environment[k] = &s
				}
			}
		case []interface{}:
			for _, item := range e {
				if s, ok := item.(string); ok {
					parts := strings.SplitN(s, "=", 2)
					if len(parts) == 2 {
						svc.Environment[parts[0]] = &parts[1]
					}
				}
			}
		}
	}

	if ports, ok := cfg["ports"].([]interface{}); ok {
		for _, p := range ports {
			switch port := p.(type) {
			case string:
				svc.Ports = append(svc.Ports, composetypes.ServicePortConfig{
					Published: port,
				})
			case map[string]interface{}:
				pc := composetypes.ServicePortConfig{}
				if target, ok := port["target"].(int); ok {
					pc.Target = uint32(target)
				}
				if published, ok := port["published"].(string); ok {
					pc.Published = published
				} else if published, ok := port["published"].(int); ok {
					pc.Published = fmt.Sprintf("%d", published)
				}
				if protocol, ok := port["protocol"].(string); ok {
					pc.Protocol = protocol
				}
				svc.Ports = append(svc.Ports, pc)
			}
		}
	}

	if volumes, ok := cfg["volumes"].([]interface{}); ok {
		for _, v := range volumes {
			switch vol := v.(type) {
			case string:
				svc.Volumes = append(svc.Volumes, composetypes.ServiceVolumeConfig{
					Source: vol,
				})
			case map[string]interface{}:
				vc := composetypes.ServiceVolumeConfig{}
				if source, ok := vol["source"].(string); ok {
					vc.Source = source
				}
				if target, ok := vol["target"].(string); ok {
					vc.Target = target
				}
				if volType, ok := vol["type"].(string); ok {
					vc.Type = volType
				}
				if ro, ok := vol["read_only"].(bool); ok {
					vc.ReadOnly = ro
				}
				svc.Volumes = append(svc.Volumes, vc)
			}
		}
	}

	if networks, ok := cfg["networks"]; ok {
		svc.Networks = make(map[string]*composetypes.ServiceNetworkConfig)
		switch nets := networks.(type) {
		case []interface{}:
			for _, n := range nets {
				if name, ok := n.(string); ok {
					svc.Networks[name] = nil
				}
			}
		case map[string]interface{}:
			for name := range nets {
				svc.Networks[name] = nil
			}
		}
	}

	if dependsOn, ok := cfg["depends_on"]; ok {
		svc.DependsOn = make(composetypes.DependsOnConfig)
		switch deps := dependsOn.(type) {
		case []interface{}:
			for _, d := range deps {
				if name, ok := d.(string); ok {
					svc.DependsOn[name] = composetypes.ServiceDependency{}
				}
			}
		case map[string]interface{}:
			for name := range deps {
				svc.DependsOn[name] = composetypes.ServiceDependency{}
			}
		}
	}

	if restart, ok := cfg["restart"].(string); ok {
		svc.Restart = restart
	}

	if hostname, ok := cfg["hostname"].(string); ok {
		svc.Hostname = hostname
	}

	if workingDir, ok := cfg["working_dir"].(string); ok {
		svc.WorkingDir = workingDir
	}

	if user, ok := cfg["user"].(string); ok {
		svc.User = user
	}

	if privileged, ok := cfg["privileged"].(bool); ok {
		svc.Privileged = privileged
	}

	return svc, nil
}

func parseNetwork(config interface{}) composetypes.NetworkConfig {
	net := composetypes.NetworkConfig{}

	if config == nil {
		return net
	}

	cfg, ok := config.(map[string]interface{})
	if !ok {
		return net
	}

	if driver, ok := cfg["driver"].(string); ok {
		net.Driver = driver
	}

	if external, ok := cfg["external"].(bool); ok {
		net.External = composetypes.External(external)
	}

	return net
}

func parseVolume(config interface{}) composetypes.VolumeConfig {
	vol := composetypes.VolumeConfig{}

	if config == nil {
		return vol
	}

	cfg, ok := config.(map[string]interface{})
	if !ok {
		return vol
	}

	if driver, ok := cfg["driver"].(string); ok {
		vol.Driver = driver
	}

	if external, ok := cfg["external"].(bool); ok {
		vol.External = composetypes.External(external)
	}

	return vol
}

func (r *ComposeResource) createNetwork(ctx context.Context, projectName, name string, config composetypes.NetworkConfig) error {
	if bool(config.External) {
		return nil // External network, don't create
	}

	networkName := fmt.Sprintf("%s_%s", projectName, name)

	// Check if network exists
	_, err := r.client.NetworkInspect(ctx, networkName, network.InspectOptions{})
	if err == nil {
		return nil // Network already exists
	}

	driver := config.Driver
	if driver == "" {
		driver = "bridge"
	}

	_, err = r.client.NetworkCreate(ctx, networkName, network.CreateOptions{
		Driver: driver,
		Labels: map[string]string{
			"com.docker.compose.project": projectName,
			"com.docker.compose.network": name,
		},
	})

	return err
}

func (r *ComposeResource) createVolume(ctx context.Context, projectName, name string, config composetypes.VolumeConfig) error {
	if bool(config.External) {
		return nil // External volume, don't create
	}

	volumeName := fmt.Sprintf("%s_%s", projectName, name)

	// Check if volume exists
	_, err := r.client.VolumeInspect(ctx, volumeName)
	if err == nil {
		return nil // Volume already exists
	}

	driver := config.Driver
	if driver == "" {
		driver = "local"
	}

	_, err = r.client.VolumeCreate(ctx, volume.CreateOptions{
		Name:   volumeName,
		Driver: driver,
		Labels: map[string]string{
			"com.docker.compose.project": projectName,
			"com.docker.compose.volume":  name,
		},
	})

	return err
}

func (r *ComposeResource) createService(ctx context.Context, projectName, serviceName string, service composetypes.ServiceConfig, project *composetypes.Project) error {
	containerName := fmt.Sprintf("%s-%s-1", projectName, serviceName)

	// Check if container exists
	_, err := r.client.ContainerInspect(ctx, containerName)
	if err == nil {
		// Container exists, start it if not running
		return r.client.ContainerStart(ctx, containerName, container.StartOptions{})
	}

	// Build container config
	containerConfig := &container.Config{
		Image: service.Image,
		Labels: map[string]string{
			"com.docker.compose.project": projectName,
			"com.docker.compose.service": serviceName,
		},
	}

	if len(service.Command) > 0 {
		containerConfig.Cmd = strslice.StrSlice(service.Command)
	}

	if len(service.Entrypoint) > 0 {
		containerConfig.Entrypoint = strslice.StrSlice(service.Entrypoint)
	}

	if service.Hostname != "" {
		containerConfig.Hostname = service.Hostname
	}

	if service.WorkingDir != "" {
		containerConfig.WorkingDir = service.WorkingDir
	}

	if service.User != "" {
		containerConfig.User = service.User
	}

	// Environment variables
	for k, v := range service.Environment {
		if v != nil {
			containerConfig.Env = append(containerConfig.Env, fmt.Sprintf("%s=%s", k, *v))
		}
	}

	// Exposed ports
	exposedPorts := nat.PortSet{}
	portBindings := nat.PortMap{}

	for _, p := range service.Ports {
		if p.Target > 0 {
			protocol := p.Protocol
			if protocol == "" {
				protocol = "tcp"
			}
			portKey := nat.Port(fmt.Sprintf("%d/%s", p.Target, protocol))
			exposedPorts[portKey] = struct{}{}
			if p.Published != "" {
				portBindings[portKey] = []nat.PortBinding{{HostPort: p.Published}}
			}
		} else if p.Published != "" {
			// Parse string format like "8080:80"
			parts := strings.Split(p.Published, ":")
			if len(parts) == 2 {
				portKey := nat.Port(fmt.Sprintf("%s/tcp", parts[1]))
				exposedPorts[portKey] = struct{}{}
				portBindings[portKey] = []nat.PortBinding{{HostPort: parts[0]}}
			}
		}
	}

	containerConfig.ExposedPorts = exposedPorts

	// Host config
	hostConfig := &container.HostConfig{
		PortBindings: portBindings,
		Privileged:   service.Privileged,
	}

	// Restart policy
	if service.Restart != "" {
		switch service.Restart {
		case "always":
			hostConfig.RestartPolicy = container.RestartPolicy{Name: container.RestartPolicyAlways}
		case "unless-stopped":
			hostConfig.RestartPolicy = container.RestartPolicy{Name: container.RestartPolicyUnlessStopped}
		case "on-failure":
			hostConfig.RestartPolicy = container.RestartPolicy{Name: container.RestartPolicyOnFailure}
		}
	}

	// Volume mounts
	for _, v := range service.Volumes {
		var m mount.Mount
		if v.Type == "volume" || v.Type == "" {
			source := v.Source
			// Check if it's a named volume from the compose file
			if _, exists := project.Volumes[v.Source]; exists {
				source = fmt.Sprintf("%s_%s", projectName, v.Source)
			}
			m = mount.Mount{
				Type:     mount.TypeVolume,
				Source:   source,
				Target:   v.Target,
				ReadOnly: v.ReadOnly,
			}
		} else if v.Type == "bind" {
			m = mount.Mount{
				Type:     mount.TypeBind,
				Source:   v.Source,
				Target:   v.Target,
				ReadOnly: v.ReadOnly,
			}
		}
		if m.Target != "" {
			hostConfig.Mounts = append(hostConfig.Mounts, m)
		}
	}

	// Network config
	networkConfig := &network.NetworkingConfig{
		EndpointsConfig: make(map[string]*network.EndpointSettings),
	}

	if len(service.Networks) > 0 {
		for netName := range service.Networks {
			fullNetName := fmt.Sprintf("%s_%s", projectName, netName)
			networkConfig.EndpointsConfig[fullNetName] = &network.EndpointSettings{}
		}
	} else {
		// Default network
		defaultNet := fmt.Sprintf("%s_default", projectName)
		networkConfig.EndpointsConfig[defaultNet] = &network.EndpointSettings{}
	}

	// Create container
	resp, err := r.client.ContainerCreate(ctx, containerConfig, hostConfig, networkConfig, nil, containerName)
	if err != nil {
		return fmt.Errorf("failed to create container: %w", err)
	}

	// Start container
	if err := r.client.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		return fmt.Errorf("failed to start container: %w", err)
	}

	return nil
}

func (r *ComposeResource) getServiceOrder(project *composetypes.Project) []string {
	// Simple topological sort based on depends_on
	visited := make(map[string]bool)
	order := make([]string, 0, len(project.Services))

	var visit func(name string)
	visit = func(name string) {
		if visited[name] {
			return
		}
		visited[name] = true

		service := project.Services[name]
		for dep := range service.DependsOn {
			visit(dep)
		}
		order = append(order, name)
	}

	// Get all service names and sort for deterministic order
	serviceNames := make([]string, 0, len(project.Services))
	for name := range project.Services {
		serviceNames = append(serviceNames, name)
	}
	sort.Strings(serviceNames)

	for _, name := range serviceNames {
		visit(name)
	}

	return order
}

func (r *ComposeResource) removeProjectContainers(ctx context.Context, projectName string) {
	containers, err := r.client.ContainerList(ctx, container.ListOptions{
		All: true,
		Filters: filters.NewArgs(
			filters.Arg("label", fmt.Sprintf("com.docker.compose.project=%s", projectName)),
		),
	})
	if err != nil {
		return
	}

	timeout := 10
	for _, c := range containers {
		_ = r.client.ContainerStop(ctx, c.ID, container.StopOptions{Timeout: &timeout})
		_ = r.client.ContainerRemove(ctx, c.ID, container.RemoveOptions{Force: true})
	}
}

func (r *ComposeResource) removeProjectNetworks(ctx context.Context, projectName string) {
	networks, err := r.client.NetworkList(ctx, network.ListOptions{
		Filters: filters.NewArgs(
			filters.Arg("label", fmt.Sprintf("com.docker.compose.project=%s", projectName)),
		),
	})
	if err != nil {
		return
	}

	for _, n := range networks {
		_ = r.client.NetworkRemove(ctx, n.ID)
	}
}

func (r *ComposeResource) removeProjectVolumes(ctx context.Context, projectName string) {
	volumes, err := r.client.VolumeList(ctx, volume.ListOptions{
		Filters: filters.NewArgs(
			filters.Arg("label", fmt.Sprintf("com.docker.compose.project=%s", projectName)),
		),
	})
	if err != nil {
		return
	}

	for _, v := range volumes.Volumes {
		_ = r.client.VolumeRemove(ctx, v.Name, true)
	}
}

func (r *ComposeResource) removeOrphanContainers(ctx context.Context, projectName string, project *composetypes.Project) {
	containers, err := r.client.ContainerList(ctx, container.ListOptions{
		All: true,
		Filters: filters.NewArgs(
			filters.Arg("label", fmt.Sprintf("com.docker.compose.project=%s", projectName)),
		),
	})
	if err != nil {
		return
	}

	timeout := 10
	for _, c := range containers {
		serviceName := c.Labels["com.docker.compose.service"]
		if _, exists := project.Services[serviceName]; !exists {
			_ = r.client.ContainerStop(ctx, c.ID, container.StopOptions{Timeout: &timeout})
			_ = r.client.ContainerRemove(ctx, c.ID, container.RemoveOptions{Force: true})
		}
	}
}

func (r *ComposeResource) calculateContentHash(data ComposeResourceModel) string {
	var content string

	if !data.ComposeFile.IsNull() {
		data, err := os.ReadFile(data.ComposeFile.ValueString())
		if err == nil {
			content = string(data)
		}
	} else if !data.ComposeContent.IsNull() {
		content = data.ComposeContent.ValueString()
	}

	hash := sha256.Sum256([]byte(content))
	return fmt.Sprintf("%x", hash)
}

func (r *ComposeResource) refreshStackInfo(ctx context.Context, data *ComposeResourceModel, project *composetypes.Project) {
	projectName := data.ProjectName.ValueString()

	// Get running containers count
	containers, err := r.client.ContainerList(ctx, container.ListOptions{
		Filters: filters.NewArgs(
			filters.Arg("label", fmt.Sprintf("com.docker.compose.project=%s", projectName)),
			filters.Arg("status", "running"),
		),
	})
	if err == nil {
		data.RunningServices = types.Int64Value(int64(len(containers)))
	}

	// Get service names from project
	if project != nil {
		serviceNames := make([]string, 0, len(project.Services))
		for name := range project.Services {
			serviceNames = append(serviceNames, name)
		}
		sort.Strings(serviceNames)
		servicesList, _ := types.ListValueFrom(ctx, types.StringType, serviceNames)
		data.Services = servicesList
	}
}

// Ensure default network exists for the project
func (r *ComposeResource) ensureDefaultNetwork(ctx context.Context, projectName string) error {
	networkName := fmt.Sprintf("%s_default", projectName)

	_, err := r.client.NetworkInspect(ctx, networkName, network.InspectOptions{})
	if err == nil {
		return nil
	}

	_, err = r.client.NetworkCreate(ctx, networkName, network.CreateOptions{
		Driver: "bridge",
		Labels: map[string]string{
			"com.docker.compose.project": projectName,
			"com.docker.compose.network": "default",
		},
	})

	return err
}

func init() {
	// Ensure time package is used
	_ = time.Second
}
