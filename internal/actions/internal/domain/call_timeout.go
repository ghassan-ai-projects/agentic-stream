package domain

import "time"

func CallTimeout(lease time.Duration) time.Duration {
	if timeout := lease - lease/10; timeout > 0 {
		return timeout
	}
	return lease
}
