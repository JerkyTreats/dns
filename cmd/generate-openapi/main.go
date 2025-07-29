package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/jerkytreats/dns/cmd/generate-openapi/analyzer"
	
	// Import packages to trigger init() functions that register routes
	_ "github.com/jerkytreats/dns/internal/api/handler"
	_ "github.com/jerkytreats/dns/internal/docs"
	_ "github.com/jerkytreats/dns/internal/healthcheck"
	_ "github.com/jerkytreats/dns/internal/tailscale/handler"
)

func main() {
	var (
		outputFile = flag.String("output", "docs/api/openapi.yaml", "Output file for OpenAPI specification")
		verbose    = flag.Bool("verbose", false, "Enable verbose logging")
	)
	flag.Parse()

	if *verbose {
		log.SetFlags(log.LstdFlags | log.Lshortfile)
	}

	log.Printf("Starting OpenAPI specification generation...")
	log.Printf("Output file: %s", *outputFile)

	// Create analyzer
	gen := analyzer.NewGenerator()

	// Generate the OpenAPI specification
	spec, err := gen.GenerateSpec()
	if err != nil {
		log.Fatalf("Failed to generate OpenAPI spec: %v", err)
	}

	// Write to output file
	if err := os.WriteFile(*outputFile, []byte(spec), 0644); err != nil {
		log.Fatalf("Failed to write spec to file: %v", err)
	}

	log.Printf("OpenAPI specification generated successfully at %s", *outputFile)
	fmt.Printf("Generated OpenAPI spec with %d routes\n", len(gen.GetDiscoveredRoutes()))
}