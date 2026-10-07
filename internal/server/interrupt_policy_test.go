package server

import (
	"errors"
	"testing"
)

func TestInspectionFailureBlocksInput(t *testing.T) {
	t.Setenv("XCAUTOKIT_INTERRUPT_GUARD", "")
	out:=inputBlockFromSnapshot("device",nil,errors.New("accessibility unavailable"))
	if out["success"]!=false || out["reason"]!="ui_unavailable" || out["performed"]!=false { t.Fatalf("unsafe result: %+v",out) }
}
