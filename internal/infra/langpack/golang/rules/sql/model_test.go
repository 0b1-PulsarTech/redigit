package sql

import "testing"

func TestTableNamesType(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		table    string
		typeName string
		want     bool
	}{
		{"channel_accounts", "ChannelAccount", true},
		{"business_hours", "BusinessHours", true},
		{"activities", "Activity", true},
		{"outboxes", "Outbox", true},
		{"quick_replies", "QuickReply", true},
		{"tags", "LeadTag", false},
		{"lead_tags", "Tag", false},
		{"users", "Roles", false},
	}

	for _, testCase := range testCases {
		t.Run(testCase.table+"/"+testCase.typeName, func(t *testing.T) {
			t.Parallel()

			if got := tableNamesType(testCase.table, testCase.typeName); got != testCase.want {
				t.Fatalf("tableNamesType() = %v, want %v", got, testCase.want)
			}
		})
	}
}
