package api

import (
	"context"
	"net/http"
	"time"
)

// extensionRoutes are the phase 4 additions: firmware, zero-touch provisioning, speech, fax sending.
func (s *Server) extensionRoutes(mux *http.ServeMux, admin, recep, full func(http.HandlerFunc) http.Handler) {
	mux.Handle("GET /api/firmware", admin(s.firmwareList))
	mux.Handle("POST /api/firmware/upload", admin(s.firmwareUpload))
	mux.Handle("POST /api/firmware/fetch", admin(s.firmwareFetch))
	mux.Handle("PUT /api/firmware/{id}", admin(s.firmwareSave))
	mux.Handle("DELETE /api/firmware/{id}", admin(s.firmwareDelete))
	mux.Handle("GET /api/firmware/latest", admin(s.firmwareLatest))
	mux.Handle("PUT /api/firmware/latest/settings", admin(s.firmwareWatchSave))
	mux.Handle("POST /api/firmware/latest/install", admin(s.firmwareLatestInstall))

	mux.Handle("GET /api/provisioning/ztp", admin(s.ztpGet))
	mux.Handle("PUT /api/provisioning/ztp", admin(s.ztpSave))
	mux.Handle("POST /api/phones/{id}/ztp", admin(s.ztpPhone))
	mux.Handle("GET /api/provisioning/ztp/export", admin(s.ztpExport))

	mux.Handle("GET /api/speech", admin(s.speechGet))
	mux.Handle("PUT /api/speech", admin(s.speechSave))
	mux.Handle("POST /api/speech/download", admin(s.speechDownload))
	mux.Handle("DELETE /api/speech/files/{id}", admin(s.speechRemove))
	mux.Handle("POST /api/speech/tts", admin(s.speechTTS))
	s.elevenRoutes(mux, admin)

	mux.Handle("GET /api/fax/send/boxes", recep(s.faxSendBoxes))
	mux.Handle("POST /api/fax/send", recep(s.faxSend))
	mux.Handle("GET /api/fax/jobs", recep(s.faxJobs))
	mux.Handle("GET /api/fax/jobs/{id}/pdf", recep(s.faxJobPDF))
	mux.Handle("POST /api/fax/jobs/{id}/retry", recep(s.faxJobRetry))
	mux.Handle("DELETE /api/fax/jobs/{id}", admin(s.faxJobDelete))
	mux.Handle("GET /api/fax/mail", admin(s.faxMailGet))
	mux.Handle("PUT /api/fax/mail", admin(s.faxMailSave))

	mux.Handle("GET /api/system/metrics", admin(s.metricsGet))
	mux.Handle("PUT /api/system/metrics", admin(s.metricsSave))
}

// RunExtensions does the periodic work of phase 4: the fax queue (every 5 s), e-mail to fax (every
// minute) and voicemail transcription and clean-up (every 2 minutes).
func (s *Server) RunExtensions(ctx context.Context) {
	select {
	case <-ctx.Done():
		return
	case <-time.After(20 * time.Second):
	}
	tick := time.NewTicker(5 * time.Second)
	defer tick.Stop()
	var n int
	for {
		s.runFaxQueue(ctx)
		if n%12 == 0 {
			go s.pollFaxMailGuarded(ctx)
		}
		if n%24 == 0 {
			go s.housekeepingGuarded(ctx)
		}
		n++
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

func (s *Server) pollFaxMailGuarded(ctx context.Context) {
	if !s.extBusy.TryLock() {
		return
	}
	defer s.extBusy.Unlock()
	s.pollFaxMail(ctx)
}

func (s *Server) housekeepingGuarded(ctx context.Context) {
	if !s.sttBusy.TryLock() {
		return
	}
	defer s.sttBusy.Unlock()
	s.purgeFaxJobs(ctx)
	tctx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()
	s.transcribeSpool(tctx)
	s.runAI(tctx)
}
