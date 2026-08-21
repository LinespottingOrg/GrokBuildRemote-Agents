package main

import (
	"testing"

	"github.com/LinespottingOrg/GrokBuildRemote-Agents/internal/core"
)

func TestApplyFleetOfferFromFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	t.Setenv("GBR_FLEET_OFFER_DIR", "")
	if _, err := core.WriteFleetOffer(core.FleetOffer{
		Name: "mac", MailboxID: "gbr-fromoffer", MailboxKey: "offer-key", OS: "darwin",
	}); err != nil {
		t.Fatal(err)
	}
	dev := core.FleetDevice{ID: "mac", Name: "mac"}
	if err := applyFleetOffer(&dev); err != nil {
		t.Fatal(err)
	}
	if dev.MailboxID != "gbr-fromoffer" || dev.MailboxKey != "offer-key" || dev.OS != "darwin" {
		t.Fatalf("got %+v", dev)
	}
}

func TestApplyFleetOfferKeepsFlags(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	dev := core.FleetDevice{ID: "mac", MailboxID: "gbr-flag", MailboxKey: "flag-key", OS: "darwin"}
	if err := applyFleetOffer(&dev); err != nil {
		t.Fatal(err)
	}
	if dev.MailboxKey != "flag-key" {
		t.Fatalf("flags should win, got %+v", dev)
	}
}

func TestApplyFleetOfferMissing(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	t.Setenv("GBR_FLEET_OFFER_DIR", dir)
	dev := core.FleetDevice{ID: "mac", Name: "mac"}
	if err := applyFleetOffer(&dev); err == nil {
		t.Fatal("expected error when no offer and no flags")
	}
}
