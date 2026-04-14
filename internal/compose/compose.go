package compose

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/cli/cli/command"
	"github.com/docker/cli/cli/flags"
	"github.com/docker/compose/v5/cmd/formatter"
	"github.com/docker/compose/v5/pkg/api"
	"github.com/docker/compose/v5/pkg/compose"
	"github.com/moby/moby/client"
	"github.com/subosito/gotenv"

	"github.com/xenforo-ltd/cli/internal/version"
)

// TODO: split up interface?
// TODO: rename NewEnv?
// TODO: fix initialisms?
// TODO: split up NewEnv?
// TODO: sensible context timeouts?
// TODO: tests?
// TODO: move some functions to be receivers?

const (
	webService = "caddy"
)

var (
	ErrNoDocker = errors.New("docker is not available")
)

type Env struct {
	dir      string
	instance string
	contexts []string
	service  api.Compose
	project  *types.Project
}

type Environment interface {
	Dir() string

	Instance() string

	Contexts() []string

	// Up starts the Docker environment. If attach is true, it will attach to the container logs.
	Up(ctx context.Context, attach bool) error

	Composer(ctx context.Context, args ...string) (int, error)

	GetUrl(ctx context.Context) (string, error)
}

func NewEnv(ctx context.Context, dir string) (Environment, error) {
	e, err := parseEnv(dir)
	if err != nil {
		return nil, err
	}

	instance, ok := e["XF_INSTANCE"]
	if !ok {
		return nil, fmt.Errorf("XF_INSTANCE not set in .env file")
	}

	contexts, ok := e["XF_CONTEXTS"]
	if !ok {
		return nil, fmt.Errorf("XF_CONTEXTS not set in .env file")
	}

	env := &Env{
		dir:      dir,
		instance: instance,
		contexts: strings.Split(contexts, ":"),
	}

	cli, err := newDockerCli(ctx)
	if err != nil {
		return nil, err
	}

	service, err := newComposeService(cli)
	if err != nil {
		return nil, err
	}

	project, err := loadProject(ctx, service, projectLoadOptions(env))
	if err != nil {
		return nil, err
	}

	env.service = service
	env.project = project

	return env, nil
}

func (env *Env) Dir() string {
	return env.dir
}

func (env *Env) Root() (*os.Root, error) {
	return os.OpenRoot(env.dir)
}

func (env *Env) Instance() string {
	return env.instance
}

func (env *Env) Contexts() []string {
	return env.contexts
}

func (env *Env) Up(ctx context.Context, attach bool) error {
	var logConsumer api.LogConsumer
	if attach {
		logConsumer = formatter.NewLogConsumer(
			ctx,
			os.Stdout,
			os.Stderr,
			true,
			true,
			true,
		)
	}

	err := env.service.Up(ctx, env.project, api.UpOptions{
		Create: api.CreateOptions{},
		Start: api.StartOptions{
			Attach: logConsumer,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to start project: %w", err)
	}

	return nil
}

func (env *Env) Composer(ctx context.Context, args ...string) (int, error) {
	code, err := env.service.RunOneOffContainer(ctx, env.project, api.RunOptions{
		Service:    "xf",
		Command:    append([]string{"composer"}, args...),
		AutoRemove: true,
	})
	if err != nil {
		return 0, fmt.Errorf("failed to run composer command: %w", err)
	}

	return code, nil
}

func (env *Env) GetUrl(ctx context.Context) (string, error) {
	domain, domainErr := env.getOrbstackDomain(ctx)
	if domainErr == nil {
		return fmt.Sprintf("https://%v", domain), nil
	}

	port, portErr := env.getPort(ctx)
	if portErr != nil {
		return "", fmt.Errorf(
			"failed to get environment URL: %w",
			errors.Join(domainErr, portErr),
		)
	}

	return fmt.Sprintf("http://localhost:%v", port), nil
}

func (env *Env) getSummary(
	ctx context.Context,
	service string,
) (*api.ContainerSummary, error) {
	summaries, err := env.service.Ps(ctx, env.project.Name, api.PsOptions{
		Services: []string{service},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get project status: %w", err)
	}

	var summary *api.ContainerSummary
	for _, s := range summaries {
		if s.Service != service {
			continue
		}

		summary = &s
		break
	}

	if summary == nil {
		return nil, fmt.Errorf("%v service not found", service)
	}

	return summary, nil
}

func (env *Env) getPort(ctx context.Context) (int, error) {
	_, port, err := env.service.Port(
		ctx,
		env.project.Name,
		webService,
		80,
		api.PortOptions{Protocol: "tcp"},
	)
	if err != nil {
		return 0, fmt.Errorf("failed to get service port: %w", err)
	}

	return port, nil
}

func (env *Env) getOrbstackDomain(ctx context.Context) (string, error) {
	summary, err := env.getSummary(ctx, webService)
	if err != nil {
		return "", err
	}

	domains, ok := summary.Labels["dev.orbstack.domains"]
	if !ok {
		return "", fmt.Errorf("orbstack domain label not found on container")
	}

	domain, _, _ := strings.Cut(domains, ",")
	if domain == "" {
		return "", fmt.Errorf("failed to parse orbstack domain from label")
	}

	return domain, nil
}

func parseEnv(dir string) (map[string]string, error) {
	r, err := os.OpenInRoot(dir, ".env")
	if err != nil {
		return nil, fmt.Errorf("failed to open .env file: %w", err)
	}

	defer r.Close()

	e, err := gotenv.StrictParse(r)
	if err != nil {
		return nil, fmt.Errorf("failed to parse .env file: %w", err)
	}

	return e, nil
}

func composeFiles(env *Env) []string {
	files := []string{filepath.Join(env.dir, "compose.yaml")}

	for _, c := range env.contexts {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}

		files = append(
			files,
			filepath.Join(env.dir, fmt.Sprintf("compose.%v.yaml", c)),
		)
	}

	return files
}

func projectLoadOptions(env *Env) api.ProjectLoadOptions {
	return api.ProjectLoadOptions{
		ProjectName: env.instance,
		ConfigPaths: composeFiles(env),
	}
}

func newDockerCli(ctx context.Context) (*command.DockerCli, error) {
	cli, err := command.NewDockerCli(
		command.WithBaseContext(ctx),
		command.WithUserAgent("XenForo-CLI/"+version.Version+" ("+runtime.GOOS+")"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create docker CLI: %w", err)
	}

	err = cli.Initialize(&flags.ClientOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to initialize docker CLI: %w", err)
	}

	_, err = cli.Client().Ping(ctx, client.PingOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to ping docker daemon: %w: %w", ErrNoDocker, err)
	}

	return cli, nil
}

func newComposeService(cli *command.DockerCli) (api.Compose, error) {
	service, err := compose.NewComposeService(cli)
	if err != nil {
		return nil, fmt.Errorf("failed to create compose service: %w", err)
	}

	return service, nil
}

func loadProject(
	ctx context.Context,
	service api.Compose,
	opts api.ProjectLoadOptions,
) (*types.Project, error) {
	project, err := service.LoadProject(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to load project: %w", err)
	}

	return project, nil
}
