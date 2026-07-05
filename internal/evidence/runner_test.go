package evidence

import (
	"context"
	"testing"
)

func TestRunCapturesSuccessfulCommandReceipt(t *testing.T) {
	receipt, err := Run(context.Background(), []string{"sh", "-c", "printf hello"}, RunOptions{})
	if err != nil {
		t.Fatalf("run command: %v", err)
	}
	if receipt.ExitCode != 0 {
		t.Fatalf("expected exit 0: %+v", receipt)
	}
	if receipt.StdoutSHA256 == "" || receipt.StderrSHA256 == "" {
		t.Fatalf("expected output hashes: %+v", receipt)
	}
	if receipt.Summary != "sh -c printf hello exited 0" {
		t.Fatalf("unexpected summary: %q", receipt.Summary)
	}
}

func TestRunReturnsReceiptForFailedCommand(t *testing.T) {
	receipt, err := Run(context.Background(), []string{"sh", "-c", "printf passed; exit 7"}, RunOptions{})
	if err != nil {
		t.Fatalf("nonzero exit should still return a receipt, not an error: %v", err)
	}
	if receipt.ExitCode != 7 {
		t.Fatalf("expected exit 7: %+v", receipt)
	}
	bundle := Bundle{Receipts: []Receipt{receipt}}
	if bundle.HasSuccessfulValidation(KindGeneric) {
		t.Fatalf("failed command receipt must not validate")
	}
}
