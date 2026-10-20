package domain

import (
	"errors"
	"fmt"
	"slices"
)

var ErrKindCommissionedTwice = errors.New("plantwire: a kind is commissioned twice")

type Inventory struct {
	digests map[string]string
	devices []string
}

func Commission(files []Kind, devices []string) (Inventory, error) {
	inventory := Inventory{digests: map[string]string{}, devices: slices.Sorted(slices.Values(devices))}
	for _, file := range files {
		if err := inventory.commission(file); err != nil {
			return Inventory{}, err
		}
	}
	return inventory, nil
}

func (i Inventory) commission(file Kind) error {
	if _, twice := i.digests[foldedName(file)]; twice {
		return fmt.Errorf("%w: %q", ErrKindCommissionedTwice, file.Name())
	}
	if err := checkKindFile(file); err != nil {
		return err
	}
	digest, err := KindDigest(file)
	if err != nil {
		return err
	}
	i.digests[foldedName(file)] = digest
	return nil
}

func (i Inventory) Quarantined(b Birth) []string {
	var quarantined []string
	for _, kind := range b.Kinds {
		if i.digests[foldedName(kind)] != kind.DeclaredDigest() {
			quarantined = append(quarantined, kind.Name())
		}
	}
	slices.Sort(quarantined)
	return quarantined
}

func (i Inventory) missingDevice(b Birth) (string, bool) {
	for _, device := range i.devices {
		if !b.carries(device) {
			return device, true
		}
	}
	return "", false
}

func misdigestedKind(b Birth) (string, bool) {
	for _, kind := range b.Kinds {
		if !kind.recomputes() {
			return kind.Name(), true
		}
	}
	return "", false
}

func repeatedKind(b Birth) (string, bool) {
	seen := map[string]bool{}
	for _, kind := range b.Kinds {
		if seen[foldedName(kind)] {
			return kind.Name(), true
		}
		seen[foldedName(kind)] = true
	}
	return "", false
}
