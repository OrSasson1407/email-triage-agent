package ai_test

import (
    "testing"
    "github.com/stretchr/testify/assert"
    "email-triage-agent/ai"
)

func TestClassificationFields(t *testing.T) {
    clf := ai.Classification{
        Urgency:     "HIGH",
        Topic:       "billing",
        Confidence:  0.95,
        ActionItems: []string{"respond within 1 hour"},
    }
    assert.Equal(t, "HIGH", clf.Urgency)
    assert.Equal(t, "billing", clf.Topic)
    assert.InDelta(t, 0.95, clf.Confidence, 0.001)
    assert.Len(t, clf.ActionItems, 1)
}
