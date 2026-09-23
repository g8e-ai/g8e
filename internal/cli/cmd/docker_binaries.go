// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package cmd

import (
	"context"
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

type dockerBinaryRunner interface {
	InspectImage(context.Context, string) (string, error)
	CreateContainer(context.Context, string) (string, error)
	CopyContainerPath(context.Context, string, string, io.Writer) error
	RemoveContainer(context.Context, string) error
}

type execDockerBinaryRunner struct{}

func (execDockerBinaryRunner) InspectImage(ctx context.Context, image string) (string, error) {
	output, err := exec.CommandContext(ctx, constants.DockerExecutable, "image", "inspect", "--format={{.Id}}", image).Output()
	if err != nil {
		return "", fmt.Errorf("inspect image %q: %w", image, err)
	}
	id := strings.TrimSpace(string(output))
	if id == "" {
		return "", fmt.Errorf("%w: image %q has no immutable ID", constants.ErrG8eBinaryExport, image)
	}
	return id, nil
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

func dockerBinariesExportCmd() *cobra.Command {
	var image, output string
	cmd := &cobra.Command{
		Use:   "binaries export",
		Short: "Export the g8e-binary set from an existing Gateway image",
		Args:  cobra.NoArgs,
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
	if err := exportDockerG8eBinaries(ctx, execDockerBinaryRunner{}, defaultDockerGatewayImage, filepath.Join(constants.PathCurrentDir, constants.BinDirname)); err != nil {
		return fmt.Errorf("%w: export g8e binaries: %w", constants.ErrG8eBinaryExport, err)
	}
	return nil
}

func exportDockerG8eBinaries(ctx context.Context, runner dockerBinaryRunner, image, output string) (err error) {
	if strings.TrimSpace(image) == "" || strings.TrimSpace(output) == "" {
		return fmt.Errorf("%w: image and output are required", constants.ErrG8eBinaryExport)
	}
	imageID, err := runner.InspectImage(ctx, image)
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrG8eBinaryExport, err)
	}
	container, err := runner.CreateContainer(ctx, imageID)
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
	manifest, err := publishDockerArchive(ctx, runner, publisher, container)
	if err != nil {
		return fmt.Errorf("%w: %w", constants.ErrG8eBinaryExport, err)
	}
	manifestDigest, err := g8ebinaries.ManifestDigest(output)
	if err != nil {
		return err
	}
	if err := g8ebinaries.WriteExportRecord(output, g8ebinaries.ExportRecord{
		ImageReference: image,
		ImageID:        imageID,
		ManifestSHA256: manifestDigest,
	}); err != nil {
		return err
	}
	_ = manifest
	return nil
}

func publishDockerArchive(ctx context.Context, runner dockerBinaryRunner, publisher *g8ebinaries.Publisher, container string) (g8ebinaries.Manifest, error) {
	reader, writer := io.Pipe()
	copyErr := make(chan error, 1)
	go func() {
		copyErr <- runner.CopyContainerPath(ctx, container, constants.G8eBinariesArchiveRoot+"/.", writer)
		_ = writer.Close()
	}()
	manifest, publishErr := publisher.Publish(reader)
	if publishErr != nil {
		_ = reader.CloseWithError(publishErr)
	}
	if err := <-copyErr; err != nil {
		return g8ebinaries.Manifest{}, err
	}
	if publishErr != nil {
		return g8ebinaries.Manifest{}, publishErr
	}
	return manifest, nil
}
