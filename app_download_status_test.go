package main

import "testing"

func TestModelDownloadStatusPersistsAndPreventsConcurrentDownload(t *testing.T) {
	app := &App{downloadStatus: ModelDownloadStatus{Status: "idle", Progress: -1}}
	if err := app.beginModelDownload(defaultLocalModel); err != nil {
		t.Fatal(err)
	}

	status := app.GetModelDownloadStatus()
	if status.Status != "downloading" || status.Model != defaultLocalModel || status.Progress != -1 {
		t.Fatalf("unexpected initial download status: %+v", status)
	}
	if err := app.beginModelDownload(defaultNLLBModel); err == nil {
		t.Fatal("expected a concurrent download to be rejected")
	}

	app.setModelDownloadStatus(ModelDownloadStatus{
		Model: defaultLocalModel, Status: "downloading", Message: "当前文件 42%", Progress: 42,
	})
	status = app.GetModelDownloadStatus()
	if status.Progress != 42 || status.Message != "当前文件 42%" || status.UpdatedAt == "" {
		t.Fatalf("download status was not retained: %+v", status)
	}
}
