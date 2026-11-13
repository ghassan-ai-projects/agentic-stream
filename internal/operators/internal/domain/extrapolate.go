package domain

import (
	"math"
	"time"

	"github.com/ghassan-ai-projects/agentic-stream/internal/spec"
)

const leastSecondsToLimit = 1

type polynomial struct{ level, rise, curvature float64 }

func secondsToLimit(e spec.Extrapolation, samples []Sample, end time.Time) float64 {
	if len(samples) < e.MinPoints {
		return 0
	}
	fit, ok := fitSamples(e.Fit, samples, end)
	if !ok || fit.rise <= 0 {
		return 0
	}
	hours, ahead := fit.hoursTo(e.Limit)
	seconds := hours * 3600
	if !ahead || e.MaxHorizon > 0 && seconds > e.MaxHorizon.Seconds() {
		return 0
	}
	return math.Max(seconds, leastSecondsToLimit)
}

func (p polynomial) hoursTo(limit float64) (float64, bool) {
	gap := limit - p.level
	if gap <= 0 {
		return 0, true
	}
	discriminant := roundedProduct(p.rise, p.rise) + roundedProduct(4, roundedProduct(p.curvature, gap))
	if discriminant < 0 {
		return 0, false
	}
	return 2 * gap / (p.rise + math.Sqrt(discriminant)), true
}

func fitSamples(fit string, samples []Sample, end time.Time) (polynomial, bool) {
	m := momentsOf(samples, end)
	if fit == spec.FitQuadratic {
		return m.quadratic()
	}
	return m.linear()
}

type moments struct{ s0, s1, s2, s3, s4, t0, t1, t2 float64 }

func momentsOf(samples []Sample, end time.Time) moments {
	var m moments
	for _, s := range samples {
		x := s.EventTime.Sub(end).Hours()
		xx := roundedProduct(x, x)
		m.s0++
		m.s1 += x
		m.s2 += xx
		m.s3 += roundedProduct(xx, x)
		m.s4 += roundedProduct(xx, xx)
		m.t0 += s.Value
		m.t1 += roundedProduct(x, s.Value)
		m.t2 += roundedProduct(xx, s.Value)
	}
	return m
}

func (m moments) linear() (polynomial, bool) {
	det := roundedProduct(m.s0, m.s2) - roundedProduct(m.s1, m.s1)
	if det == 0 {
		return polynomial{}, false
	}
	level := (roundedProduct(m.t0, m.s2) - roundedProduct(m.s1, m.t1)) / det
	rise := (roundedProduct(m.s0, m.t1) - roundedProduct(m.s1, m.t0)) / det
	return polynomial{level: level, rise: rise}, true
}

func (m moments) quadratic() (polynomial, bool) {
	det := determinant(m.s0, m.s1, m.s2, m.s1, m.s2, m.s3, m.s2, m.s3, m.s4)
	if det == 0 {
		return polynomial{}, false
	}
	return polynomial{
		level:     determinant(m.t0, m.s1, m.s2, m.t1, m.s2, m.s3, m.t2, m.s3, m.s4) / det,
		rise:      determinant(m.s0, m.t0, m.s2, m.s1, m.t1, m.s3, m.s2, m.t2, m.s4) / det,
		curvature: determinant(m.s0, m.s1, m.t0, m.s1, m.s2, m.t1, m.s2, m.s3, m.t2) / det,
	}, true
}

func determinant(a, b, c, d, e, f, g, h, i float64) float64 {
	return roundedProduct(a, roundedProduct(e, i)-roundedProduct(f, h)) -
		roundedProduct(b, roundedProduct(d, i)-roundedProduct(f, g)) +
		roundedProduct(c, roundedProduct(d, h)-roundedProduct(e, g))
}
