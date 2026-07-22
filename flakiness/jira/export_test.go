package jira

import "time"

// SetClientClock replaces the clock used by CreateBug. Test use only.
func SetClientClock(c *Client, fn func() time.Time) {
	c.now = fn
}
