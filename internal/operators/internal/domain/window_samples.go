package domain

import "slices"

type sampleChanges struct {
	admitted []Sample
	expired  []Sample
}

func (c *sampleChanges) admit(sample Sample) {
	c.admitted = append(c.admitted, sample)
}

func (c *sampleChanges) expire(sample Sample) {
	if at := slices.IndexFunc(c.admitted, sameSample(sample)); at >= 0 {
		c.admitted = slices.Delete(c.admitted, at, at+1)
		return
	}
	c.expired = append(c.expired, sample)
}

func sameSample(want Sample) func(Sample) bool {
	return func(got Sample) bool { return got.EventID == want.EventID && got.EventTime.Equal(want.EventTime) }
}

func (ws *WindowState) UnsavedSamples() (admitted, expired []Sample) {
	return ws.changes.admitted, ws.changes.expired
}

func (ws *WindowState) SamplesSaved() {
	ws.changes = sampleChanges{}
}

func (ws *WindowState) RestoreSamples(samples []Sample) {
	ws.Samples = samples
	slices.SortStableFunc(ws.Samples, compareSamples)
	ws.changes = sampleChanges{}
}
