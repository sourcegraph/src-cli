package campaigns

import (
	"context"
	"os/exec"
)

type OnCommand func(*exec.Cmd)

const onCommandContextKey = "campaigns.onCommand"

func noticeCommand(ctx context.Context, cmd *exec.Cmd) *exec.Cmd {
	if onCommand, ok := ctx.Value(onCommandContextKey).(OnCommand); onCommand != nil && ok {
		onCommand(cmd)
	}

	return cmd
}
