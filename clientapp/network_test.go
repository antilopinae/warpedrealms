// Copyright (c) 2024 Warped Realms. All rights reserved.
// This source code is proprietary and confidential.
// Unauthorized copying or cloning of game mechanics is strictly prohibited.
// See LICENSE file in the project root for full license details.

package clientapp

import (
	"net/url"
	"testing"

	"warpedrealms/shared"
)

func TestWebsocketURLIncludesRaidAndClass(t *testing.T) {
	client := NewNetworkClient("http://127.0.0.1:8080")

	rawURL, err := client.websocketURL("token-123", "raid-007", shared.PlayerClassKnight)
	if err != nil {
		t.Fatalf("websocketURL failed: %v", err)
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse websocket URL: %v", err)
	}
	if parsed.Scheme != "ws" {
		t.Fatalf("expected ws scheme, got %s", parsed.Scheme)
	}
	if parsed.Path != "/ws" {
		t.Fatalf("expected /ws path, got %s", parsed.Path)
	}
	query := parsed.Query()
	if query.Get("token") != "token-123" {
		t.Fatalf("expected token query param, got %s", query.Get("token"))
	}
	if query.Get("raid") != "raid-007" {
		t.Fatalf("expected raid query param, got %s", query.Get("raid"))
	}
	if query.Get("class") != shared.PlayerClassKnight {
		t.Fatalf("expected class query param %s, got %s", shared.PlayerClassKnight, query.Get("class"))
	}
}
