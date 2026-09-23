// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmd

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	g8ebinaries "github.com/g8e-ai/g8e/v2/internal/services/g8ebinaries"
)

const defaultDockerGatewayImage = "g8e-gateway"

type dockerImage struct {
	ID           string            `json:"Id"`
	Os           string            `json:"Os"`
	Architecture string            `json:"Architecture"`
	Config       dockerImageConfig `json:"Config"`
}

type dockerImageConfig struct {
	Labels map[string]string `json:"Labels"`
}

type dockerBinaryRunner interface {
	InspectImage(context.Context, string) (dockerImage, error)
	CreateContainer(context.Context, string) (string, error)
	CopyContainerPath(context.Context, string, string, io.Writer) error
	RemoveContainer(context.Context, string) error
}

const dockerRuntimeBinaryPath = "/g8e"

type execDockerBinaryRunner struct{}

func (execDockerBinaryRunner) InspectImage(ctx context.Context, image string) (dockerImage, error) {
	output, err := exec.CommandContext(ctx, constants.DockerExecutable, "image", "inspect", "--format={{json .}}", image).Output()
	if err != nil {
		return dockerImage{}, fmt.Errorf("inspect image %q: %w", image, err)
	}
	var inspected dockerImage
	if err := json.Unmarshal(output, &inspected); err != nil {
		return dockerImage{}, fmt.Errorf("decode image inspection for %q: %w", image, err)
	}
	if strings.TrimSpace(inspected.ID) == "" {
		return dockerImage{}, fmt.Errorf("%w: image %q has no immutable ID", constants.ErrG8eBinaryExport, image)
	}
	return inspected, nil
}

func (execDockerBinaryRunner) CreateContainer(ctx context.Context, image string) (string, error) {
	output, err := exec.CommandContext(ctx, constants.DockerExecutable, "create", image).Output()
	if err != nil {
		return "", fmt.Errorf("create export container from %q: %w", image, err)
	}
	container := strings.TrimSpace(string(output))
	if container == "" {
		return "", fmt.Errorf("%w: Docker returned an empty container ID", constants.ErrG8eBinaryExport)
	}
	return container, nil
}

func (execDockerBinaryRunner) CopyContainerPath(ctx context.Context, container, path string, destination io.Writer) error {
	command := exec.CommandContext(ctx, constants.DockerExecutable, "cp", container+":"+path, "-")
	command.Stdout = destination
	command.Stderr = os.Stderr
	if err := command.Run(); err != nil {
		return fmt.Errorf("copy g8e binaries from container %q: %w", container, err)
	}
	return nil
}

func (execDockerBinaryRunner) RemoveContainer(ctx context.Context, container string) error {
	if err := exec.CommandContext(ctx, constants.DockerExecutable, "rm", container).Run(); err != nil {
		return fmt.Errorf("remove export container %q: %w", container, err)
	}
	return nil
}

func dockerBinariesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "binaries",
		Short: "Manage Gateway image g8e-binary artifacts",
	}
	cmd.AddCommand(dockerBinariesExportCmd())
	return cmd
}

func dockerBinariesExportCmd() *cobra.Command {
	var image, output string
	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export the full g8e-binary set from an existing Gateway image",
		Long: `Export the complete platform deployment matrix from a Gateway image.

Standard Gateway images only contain the runtime binary for the image target
platform. Use ` + "`make build-all`" + ` on the host when you need the full Linux,
Windows, and macOS artifact set.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := checkDockerComposeFileExists(); err != nil {
				return err
			}
			if err := checkDockerAvailable(); err != nil {
				return err
			}
			if image == "" {
				image = defaultDockerGatewayImage
			}
			if output == "" {
				cwd, err := os.Getwd()
				if err != nil {
					return fmt.Errorf("%w: resolve export output: %w", constants.ErrG8eBinaryExport, err)
				}
				output = filepath.Join(cwd, constants.BinDirname)
			}
			if err := exportDockerG8eBinaries(cmd.Context(), execDockerBinaryRunner{}, image, output); err != nil {
				return err
			}
			cmd.Printf("Exported g8e binaries from %s to %s.\n", image, output)
			return nil
		},
	}
	cmd.Flags().StringVar(&image, "image", defaultDockerGatewayImage, "Gateway image reference to export")
	cmd.Flags().StringVar(&output, "output", "", "Host directory for the exported g8e-binary mirror")
	return cmd
}

func buildDockerImagesAndExport(ctx context.Context, buildArgs []string, profiles ...string) error {
	if err := runDockerCompose(buildArgs, profiles...); err != nil {
		return fmt.Errorf("%w: build images: %w", constants.ErrProcessStartFailed, err)
	}
	if err := exportDockerRuntimeBinary(ctx, execDockerBinaryRunner{}, defaultDockerGatewayImage, filepath.Join(constants.PathCurrentDir, "g8e")); err != nil {
		return fmt.Errorf("%w: export runtime binary: %w", constants.ErrG8eBinaryExport, err)
	}
	return nil
}

func exportDockerRuntimeBinary(ctx context.Context, runner dockerBinaryRunner, image, destination string) (err error) {
	if strings.TrimSpace(image) == "" || strings.TrimSpace(destination) == "" {
		return fmt.Errorf("%w: image and destination are required", constants.ErrG8eBinaryExport)
	}
	inspected, err := runner.InspectImage(ctx, image)
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrG8eBinaryExport, err)
	}
	container, err := runner.CreateContainer(ctx, inspected.ID)
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrG8eBinaryExport, err)
	}
	defer func() {
		cleanupErr := runner.RemoveContainer(ctx, container)
		if err == nil && cleanupErr != nil {
			err = fmt.Errorf("%w: cleanup export container: %w", constants.ErrG8eBinaryExport, cleanupErr)
		}
	}()

	var archive bytes.Buffer
	if err := runner.CopyContainerPath(ctx, container, dockerRuntimeBinaryPath, &archive); err != nil {
		return fmt.Errorf("%w: %w", constants.ErrG8eBinaryExport, err)
	}
	reader := tar.NewReader(&archive)
	header, err := reader.Next()
	if err != nil {
		return fmt.Errorf("%w: read runtime binary archive: %w", constants.ErrG8eBinaryArchive, err)
	}
	if header.Name != filepath.Base(dockerRuntimeBinaryPath) || !header.FileInfo().Mode().IsRegular() || header.Size <= 0 {
		return fmt.Errorf("%w: invalid runtime binary entry %q", constants.ErrG8eBinaryArchive, header.Name)
	}
	var binary bytes.Buffer
	if _, err := io.CopyN(&binary, reader, header.Size); err != nil {
		return fmt.Errorf("%w: read runtime binary: %w", constants.ErrG8eBinaryArchive, err)
	}
	if _, err := reader.Next(); err != io.EOF {
		return fmt.Errorf("%w: runtime binary archive contains additional entries", constants.ErrG8eBinaryArchive)
	}
	staging := destination + ".new"
	if err := os.WriteFile(staging, binary.Bytes(), constants.PermFileExecutable); err != nil {
		return fmt.Errorf("%w: write runtime binary: %w", constants.ErrG8eBinaryExport, err)
	}
	if err := os.Rename(staging, destination); err != nil {
		_ = os.Remove(staging)
		return fmt.Errorf("%w: publish runtime binary: %w", constants.ErrG8eBinaryExport, err)
	}
	return nil
}

func exportDockerG8eBinaries(ctx context.Context, runner dockerBinaryRunner, image, output string) (err error) {
	if strings.TrimSpace(image) == "" || strings.TrimSpace(output) == "" {
		return fmt.Errorf("%w: image and output are required", constants.ErrG8eBinaryExport)
	}
	inspected, err := runner.InspectImage(ctx, image)
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrG8eBinaryExport, err)
	}
	provenance, err := imageProvenance(inspected)
	if err != nil {
		return err
	}
	container, err := runner.CreateContainer(ctx, inspected.ID)
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrG8eBinaryExport, err)
	}
	defer func() {
		cleanupErr := runner.RemoveContainer(ctx, container)
		if err == nil && cleanupErr != nil {
			err = fmt.Errorf("%w: cleanup export container: %w", constants.ErrG8eBinaryExport, cleanupErr)
		}
	}()

	publisher := g8ebinaries.NewPublisher(output)
	manifest, err := publishDockerArchive(ctx, runner, publisher, container, provenance)
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrG8eBinaryExport, err)
	}
	manifestDigest, err := g8ebinaries.ManifestDigest(output)
	if err != nil {
		return err
	}
	if err := g8ebinaries.WriteExportRecord(output, g8ebinaries.ExportRecord{
		ImageReference: image,
		ImageID:        inspected.ID,
		ManifestSHA256: manifestDigest,
	}); err != nil {
		return err
	}
	_ = manifest
	return nil
}

func imageProvenance(image dockerImage) (g8ebinaries.Provenance, error) {
	labels := image.Config.Labels
	provenance := g8ebinaries.Provenance{
		Version:        labels[constants.G8eImageVersionLabel],
		BuildID:        labels[constants.G8eBuildIDLabel],
		BuildTime:      labels[constants.G8eBuildTimeLabel],
		SourceRevision: labels[constants.G8eSourceRevisionLabel],
		SourceTreeHash: labels[constants.G8eSourceTreeHashLabel],
	}
	if provenance.Version == "" || provenance.BuildID == "" || provenance.BuildTime == "" || provenance.SourceRevision == "" || provenance.SourceTreeHash == "" {
		return g8ebinaries.Provenance{}, fmt.Errorf("%w: image is missing g8e provenance labels", constants.ErrG8eBinaryExport)
	}
	return provenance, nil
}

func publishDockerArchive(ctx context.Context, runner dockerBinaryRunner, publisher *g8ebinaries.Publisher, container string, provenance g8ebinaries.Provenance) (g8ebinaries.Manifest, error) {
	reader, writer := io.Pipe()
	copyErr := make(chan error, 1)
	go func() {
		copyErr <- runner.CopyContainerPath(ctx, container, constants.G8eBinariesArchiveRoot+"/.", writer)
		_ = writer.Close()
	}()
	manifest, publishErr := publisher.PublishMatching(reader, provenance)
	if publishErr != nil {
		_ = reader.CloseWithError(publishErr)
	}
	copyPathErr := <-copyErr
	if publishErr != nil {
		return g8ebinaries.Manifest{}, publishErr
	}
	if copyPathErr != nil {
		return g8ebinaries.Manifest{}, copyPathErr
	}
	return manifest, nil
}
