package environment

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
)

// containerInfo describes a single container as rendered by `xf ps`.
//
// Status and Ports are display strings derived from the Compose record rather
// than wire fields: `docker compose ps --format json` reports State, Health,
// ExitCode and Publishers, not the combined Status/Ports strings.
type containerInfo struct {
	Service string
	Name    string
	State   string
	Status  string
	Ports   string
}

// composeContainerRecord mirrors the fields of the official
// `docker compose ps --format json` schema that `xf ps` renders. Only the
// fields the table needs are decoded; unknown fields are ignored.
type composeContainerRecord struct {
	Service    string                 `json:"Service"`
	Name       string                 `json:"Name"`
	State      string                 `json:"State"`
	Health     string                 `json:"Health"`
	ExitCode   int                    `json:"ExitCode"`
	Publishers []composePortPublisher `json:"Publishers"`
}

// composePortPublisher mirrors one entry of the Compose Publishers array.
type composePortPublisher struct {
	URL           string `json:"URL"`
	TargetPort    int    `json:"TargetPort"`
	PublishedPort int    `json:"PublishedPort"`
	Protocol      string `json:"Protocol"`
}

// unknownValue stands in for a value Compose does not report, so a table cell
// is visibly unknown rather than blank.
const unknownValue = "—"

// normalize converts a wire record into the display model used by `xf ps`.
func (rec composeContainerRecord) normalize() containerInfo {
	return containerInfo{
		Service: rec.Service,
		Name:    rec.Name,
		State:   rec.State,
		Status:  rec.statusText(),
		Ports:   rec.portsText(),
	}
}

// statusText renders a container's status from the fields Compose supplies.
// Health is the most specific signal; an exited or dead container reports how
// it ended; anything else has no status beyond its state.
func (rec composeContainerRecord) statusText() string {
	if rec.Health != "" {
		return rec.Health
	}

	if rec.State == "exited" || rec.State == "dead" {
		return fmt.Sprintf("exit %d", rec.ExitCode)
	}

	return unknownValue
}

// portsText renders the container's published ports in Docker's familiar
// "host:published->target/proto" form, joining multiple mappings with commas.
func (rec composeContainerRecord) portsText() string {
	if len(rec.Publishers) == 0 {
		return unknownValue
	}

	mappings := make([]string, 0, len(rec.Publishers))
	for _, publisher := range rec.Publishers {
		mappings = append(mappings, publisher.text())
	}

	return strings.Join(mappings, ", ")
}

// text renders a single port mapping. A mapping with no published port is
// exposed but not forwarded to the host.
func (pub composePortPublisher) text() string {
	target := strconv.Itoa(pub.TargetPort) + "/" + pub.Protocol
	if pub.PublishedPort == 0 {
		return target
	}

	return net.JoinHostPort(pub.URL, strconv.Itoa(pub.PublishedPort)) + "->" + target
}

// PSInfo returns container status parsed from `docker compose ps --format json`.
// Docker stderr is included in errors when available to preserve diagnostics.
func (r *Runner) PSInfo(ctx context.Context) ([]containerInfo, error) {
	cmd := r.composeCommand(ctx, "ps", "--format", "json")

	var stdout, stderr bytes.Buffer

	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if detail := bytes.TrimSpace(stderr.Bytes()); len(detail) > 0 {
			return nil, contextError(ctx, fmt.Errorf("docker command failed: %w: %s", err, detail))
		}

		return nil, contextError(ctx, fmt.Errorf("docker command failed: %w", err))
	}

	return parseContainerInfo(stdout.Bytes())
}

// parseContainerInfo decodes `docker compose ps --format json` output.
//
// Compose has emitted two top-level shapes: a single JSON array in older
// releases, and JSON Lines (one container per line) in newer ones. Both are
// accepted; the records themselves use the official schema, whose status and
// port information lives in Health, ExitCode and Publishers.
func parseContainerInfo(data []byte) ([]containerInfo, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return []containerInfo{}, nil
	}

	var records []composeContainerRecord

	if trimmed[0] == '[' {
		if err := json.Unmarshal(trimmed, &records); err != nil {
			return nil, fmt.Errorf("failed to parse container status: %w", err)
		}
	} else {
		decoder := json.NewDecoder(bytes.NewReader(trimmed))

		for {
			var record composeContainerRecord

			err := decoder.Decode(&record)
			if errors.Is(err, io.EOF) {
				break
			}

			if err != nil {
				return nil, fmt.Errorf("failed to parse container status: %w", err)
			}

			records = append(records, record)
		}
	}

	containers := make([]containerInfo, 0, len(records))
	for _, record := range records {
		containers = append(containers, record.normalize())
	}

	return containers, nil
}
