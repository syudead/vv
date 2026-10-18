package domain

import "time"

// CopySeekAllowance covers the default 250-frame x264/x265 keyframe interval (about 10.4s at 23.976fps) with a 15s allowance.
const CopySeekAllowance = 15 * time.Second

// CopySeekWithinAllowance accepts later starts; starts earlier than requested by more than CopySeekAllowance require re-encoding.
func CopySeekWithinAllowance(requestedMs, actualMs int64) bool {
	return requestedMs-actualMs <= CopySeekAllowance.Milliseconds()
}
