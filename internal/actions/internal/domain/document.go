package domain

// Document is a decoded outcome JSON object. The original map is kept because
// canonical digests depend on its exact fields.
type Document map[string]any
