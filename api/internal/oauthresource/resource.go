// Package oauthresource exposes this service's versioned OAuth scope manifest
// and RFC 9728 Protected Resource Metadata.
package oauthresource

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/gofiber/fiber/v3"
)

//go:embed scope-manifest.json
var manifestJSON []byte

type Scope struct {
	Name         string            `json:"name"`
	Descriptions map[string]string `json:"descriptions"`
	Visibility   string            `json:"visibility"`
	Status       string            `json:"status"`
}

type Manifest struct {
	SchemaVersion    int     `json:"schema_version"`
	ResourceServerID string  `json:"resource_server_id"`
	DisplayName      string  `json:"display_name"`
	Scopes           []Scope `json:"scopes"`
}

func ManifestDocument() (Manifest, error) {
	var manifest Manifest
	if err := json.Unmarshal(manifestJSON, &manifest); err != nil {
		return Manifest{}, fmt.Errorf("decode embedded OAuth scope manifest: %w", err)
	}
	return manifest, nil
}

func PublicActiveScopes() ([]string, error) {
	manifest, err := ManifestDocument()
	if err != nil {
		return nil, err
	}
	var result []string
	for _, scope := range manifest.Scopes {
		if scope.Visibility == "public" && scope.Status == "active" {
			result = append(result, scope.Name)
		}
	}
	sort.Strings(result)
	return result, nil
}

func Register(app *fiber.App, resource, authorizationServer string) {
	app.Get("/.well-known/oauth-protected-resource", func(c fiber.Ctx) error {
		scopes, err := PublicActiveScopes()
		if err != nil {
			return fiber.ErrInternalServerError
		}
		return c.JSON(fiber.Map{
			"resource": resource, "authorization_servers": []string{authorizationServer},
			"scopes_supported": scopes,
		})
	})
}
