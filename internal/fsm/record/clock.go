package record

import "time"

func nowNanos() int64 { return time.Now().UnixNano() }
