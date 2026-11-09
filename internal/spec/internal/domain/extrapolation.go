package domain

import (
	"encoding/json"
	"fmt"
	"time"
)

const (
	FitLinear    = "linear"
	FitQuadratic = "quadratic"
)

type Extrapolation struct {
	Limit      float64
	Fit        string
	MinPoints  int
	MaxHorizon time.Duration
}

func (op Operator) Extrapolation() (Extrapolation, error) {
	e := Extrapolation{Fit: FitLinear}
	var err error
	if e.Limit, err = configuredNumber(op.Configuration, "limit"); err != nil {
		return Extrapolation{}, err
	}
	if fit, ok := op.Configuration["fit"]; ok {
		e.Fit = fmt.Sprint(fit)
	}
	if e.MinPoints, err = configuredPoints(op.Configuration, e.Fit); err != nil {
		return Extrapolation{}, err
	}
	e.MaxHorizon, err = configuredHorizon(op.Configuration)
	return e, err
}

func configuredPoints(configuration map[string]any, fit string) (int, error) {
	least, ok := map[string]int{FitLinear: 2, FitQuadratic: 3}[fit]
	if !ok {
		return 0, fmt.Errorf("fit %q is not %s or %s", fit, FitLinear, FitQuadratic)
	}
	if _, set := configuration["minPoints"]; !set {
		return least, nil
	}
	points, err := configuredNumber(configuration, "minPoints")
	if err != nil {
		return 0, err
	}
	if points != float64(int(points)) || int(points) < least {
		return 0, fmt.Errorf("minPoints %v is not a whole number of at least %d for a %s fit", points, least, fit)
	}
	return int(points), nil
}

func configuredHorizon(configuration map[string]any) (time.Duration, error) {
	raw, ok := configuration["maxHorizon"]
	if !ok {
		return 0, nil
	}
	horizon, err := ParseDuration(fmt.Sprint(raw))
	if err != nil {
		return 0, fmt.Errorf("maxHorizon: %w", err)
	}
	if horizon <= 0 {
		return 0, fmt.Errorf("maxHorizon %s is not positive", horizon)
	}
	return horizon, nil
}

func configuredNumber(configuration map[string]any, key string) (float64, error) {
	switch value := configuration[key].(type) {
	case float64:
		return value, nil
	case int:
		return float64(value), nil
	case int64:
		return float64(value), nil
	case uint64:
		return float64(value), nil
	case json.Number:
		return numberOf(key, value)
	}
	return 0, fmt.Errorf("%s is not a number", key)
}

func numberOf(key string, value json.Number) (float64, error) {
	number, err := value.Float64()
	if err != nil {
		return 0, fmt.Errorf("%s: %w", key, err)
	}
	return number, nil
}

func checkExtrapolation(op Operator) error {
	if op.Kind != "extrapolate" {
		return nil
	}
	if op.Window == "" || op.Field == "" {
		return &CompileError{Path: fmt.Sprintf("operators.%s", op.Name), Message: "an extrapolate operator needs a window and a field"}
	}
	if _, err := op.Extrapolation(); err != nil {
		return &CompileError{Path: fmt.Sprintf("operators.%s.configuration", op.Name), Message: err.Error()}
	}
	return nil
}
