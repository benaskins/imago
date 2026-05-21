package main

import "testing"

func TestParseArgs_Interview_DefaultAudience(t *testing.T) {
	period, audience, ws := parseArgs(nil)
	if period != "" {
		t.Errorf("period: got %q want empty", period)
	}
	if audience != "self" {
		t.Errorf("audience: got %q want self", audience)
	}
	if ws != "" {
		t.Errorf("workspace: got %q want empty", ws)
	}
}

func TestParseArgs_Interview_CustomAudience(t *testing.T) {
	period, audience, _ := parseArgs([]string{"--audience", "manager"})
	if period != "" {
		t.Errorf("period: got %q want empty", period)
	}
	if audience != "manager" {
		t.Errorf("audience: got %q want manager", audience)
	}
}

func TestParseArgs_Daily_DefaultAudience(t *testing.T) {
	period, audience, ws := parseArgs([]string{"daily", "/tmp/ws"})
	if period != "daily" {
		t.Errorf("period: got %q want daily", period)
	}
	if audience != "self" {
		t.Errorf("audience: got %q want self", audience)
	}
	if ws != "/tmp/ws" {
		t.Errorf("workspace: got %q want /tmp/ws", ws)
	}
}

func TestParseArgs_Daily_CustomAudience(t *testing.T) {
	period, audience, ws := parseArgs([]string{"daily", "--audience", "manager", "/tmp/ws"})
	if period != "daily" {
		t.Errorf("period: got %q want daily", period)
	}
	if audience != "manager" {
		t.Errorf("audience: got %q want manager", audience)
	}
	if ws != "/tmp/ws" {
		t.Errorf("workspace: got %q want /tmp/ws", ws)
	}
}

func TestParseArgs_Weekly(t *testing.T) {
	period, audience, ws := parseArgs([]string{"weekly", "--audience", "manager", "/tmp/ws"})
	if period != "weekly" {
		t.Errorf("period: got %q want weekly", period)
	}
	if audience != "manager" {
		t.Errorf("audience: got %q want manager", audience)
	}
	if ws != "/tmp/ws" {
		t.Errorf("workspace: got %q want /tmp/ws", ws)
	}
}
