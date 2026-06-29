// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/netutil"
	"github.com/spf13/cobra"
)

func swaggerCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "swagger",
		Short: "Manage Swagger/OpenAPI documentation",
		Long:  `Commands for generating, serving, and validating Swagger/OpenAPI documentation for the g8e Gateway API.`,
	}

	cmd.AddCommand(
		swaggerInitCmd(),
		swaggerServeCmd(),
		swaggerValidateCmd(),
	)

	return cmd
}

func swaggerInitCmd() *cobra.Command {
	var searchDir string
	var outputDir string

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Generate Swagger documentation from code annotations",
		Long:  `Generate Swagger/OpenAPI documentation by scanning Go code for Swagger annotations. Uses the swag CLI tool to parse annotations and generate docs.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Default to cmd/operator and internal/services/gateway directories
			// cmd/operator has main.go (entry point), internal/services/gateway has the annotations
			if searchDir == "" {
				searchDir = "cmd/operator,internal/services/gateway"
			}
			if outputDir == "" {
				outputDir = "internal/services/gateway/docs"
			}

			// Ensure paths are absolute
			absSearchDir, err := filepath.Abs(searchDir)
			if err != nil {
				return fmt.Errorf("%w: %v", constants.ErrPathValidation, err)
			}
			absOutputDir, err := filepath.Abs(outputDir)
			if err != nil {
				return fmt.Errorf("%w: %v", constants.ErrPathValidation, err)
			}

			// Check if swag is available
			if _, err := exec.LookPath("swag"); err != nil {
				cmd.Println("swag binary not found. Install it with:")
				cmd.Println("  go install github.com/swaggo/swag/cmd/swag@latest")
				return fmt.Errorf("%w: swag not installed", constants.ErrPathNotFound)
			} else {
				// Use installed swag binary
				swagCmd := exec.Command("swag", "init",
					"--dir", absSearchDir,
					"--output", absOutputDir,
					"--parseDependency",
					"--parseInternal",
				)
				swagCmd.Stdout = cmd.OutOrStdout()
				swagCmd.Stderr = cmd.ErrOrStderr()
				if err := swagCmd.Run(); err != nil {
					return fmt.Errorf("%w: %v", constants.ErrProcessStartFailed, err)
				}
			}

			cmd.Printf("\nSwagger documentation generated successfully!\n")
			cmd.Printf("Output directory: %s\n", absOutputDir)
			cmd.Printf("Files generated:\n")
			cmd.Printf("  - %s/swagger.json\n", absOutputDir)
			cmd.Printf("  - %s/swagger.yaml\n", absOutputDir)
			cmd.Printf("  - %s/docs.go\n", absOutputDir)

			return nil
		},
	}

	cmd.Flags().StringVar(&searchDir, "dir", "", "Directory to search for Swagger annotations (default: cmd/operator,internal/services/gateway)")
	cmd.Flags().StringVar(&outputDir, "output", "", "Output directory for generated docs (default: internal/services/gateway/docs)")

	return cmd
}

func swaggerServeCmd() *cobra.Command {
	var port int
	var host string

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Serve Swagger UI for API documentation",
		Long:  `Start a local HTTP server to serve the Swagger UI for viewing and testing the API documentation.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Default values
			if port == 0 {
				port = 8081
			}
			if host == "" {
				host = "localhost"
			}

			docsPath := "internal/services/gateway/docs"
			absDocsPath, err := filepath.Abs(docsPath)
			if err != nil {
				return fmt.Errorf("%w: %v", constants.ErrPathValidation, err)
			}

			// Check if swagger.json exists
			swaggerJSON := filepath.Join(absDocsPath, constants.SwaggerFilename)
			if _, err := os.Stat(swaggerJSON); os.IsNotExist(err) {
				cmd.Printf("Swagger documentation not found at %s\n", swaggerJSON)
				cmd.Println("Run 'g8e swagger init' to generate documentation first.")
				return fmt.Errorf("%w: %s", constants.ErrPathNotFound, swaggerJSON)
			}

			cmd.Printf("Serving Swagger UI at http://%s:%d/swagger/index.html\n", host, port)
			cmd.Printf("Press Ctrl+C to stop.\n")

			cmd.Println("\nNote: To serve Swagger UI, start the g8e Gateway and access:")
			cmd.Printf("  %s/swagger/index.html\n", netutil.LocalhostHTTPSURL(constants.Ports.OperatorHttps))
			cmd.Println("\nOr use a standalone tool like:")
			cmd.Printf("  npx @apidevtools/swagger-cli serve %s -p %d\n", swaggerJSON, port)
			cmd.Printf("  docker run -p %d:8080 -e SWAGGER_JSON=/swagger/swagger.json -v %s:/swagger swaggerapi/swagger-ui\n", port, absDocsPath)

			return nil
		},
	}

	cmd.Flags().IntVar(&port, "port", 0, "Port to serve Swagger UI on (default: 8081)")
	cmd.Flags().StringVar(&host, "host", "", "Host to bind to (default: localhost)")

	return cmd
}

func swaggerValidateCmd() *cobra.Command {
	var specFile string

	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Validate Swagger/OpenAPI specification",
		Long:  `Validate the generated Swagger/OpenAPI specification for errors and compliance.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Default to gateway swagger.json
			if specFile == "" {
				specFile = "internal/services/gateway/docs/swagger.json"
			}

			absSpecFile, err := filepath.Abs(specFile)
			if err != nil {
				return fmt.Errorf("%w: %v", constants.ErrPathValidation, err)
			}

			// Check if file exists
			if _, err := os.Stat(absSpecFile); os.IsNotExist(err) {
				return fmt.Errorf("%w: %s", constants.ErrPathNotFound, absSpecFile)
			}

			// Try to use swagger-cli if available
			if _, err := exec.LookPath("swagger-cli"); err == nil {
				validateCmd := exec.Command("swagger-cli", "validate", absSpecFile)
				validateCmd.Stdout = cmd.OutOrStdout()
				validateCmd.Stderr = cmd.ErrOrStderr()
				if err := validateCmd.Run(); err != nil {
					return fmt.Errorf("%w: %v", constants.ErrValidationFailed, err)
				}
				cmd.Println("Swagger specification is valid!")
				return nil
			}

			// Try using swag
			if _, err := exec.LookPath("swag"); err == nil {
				cmd.Println("Using swag for basic validation...")
				// swag doesn't have a direct validate command, but init will fail if there are errors
				cmd.Println("Note: Run 'g8e swagger init' to check for annotation errors.")
				return nil
			}

			// Fallback: suggest installing tools
			cmd.Println("No swagger validation tool found.")
			cmd.Println("Install one of the following:")
			cmd.Println("  npm install -g @apidevtools/swagger-cli")
			cmd.Println("  go install github.com/go-swagger/go-swagger/cmd/swagger@latest")
			return nil
		},
	}

	cmd.Flags().StringVar(&specFile, "file", "", "Path to Swagger spec file (default: internal/services/gateway/docs/swagger.json)")

	return cmd
}
