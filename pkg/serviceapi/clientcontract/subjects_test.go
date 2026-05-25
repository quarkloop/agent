package clientcontract

import "testing"

func TestClientSubjectsAreStable(t *testing.T) {
	if SubjectSpaceCreate != "control.space.v1.create" {
		t.Fatalf("space create subject = %q", SubjectSpaceCreate)
	}
	if SubjectAuditGet != "control.audit.v1.get" || SubjectAuditList != "control.audit.v1.list" {
		t.Fatalf("audit subjects = %q, %q", SubjectAuditGet, SubjectAuditList)
	}
	subject, err := SessionInputSubject("Chat-01")
	if err != nil {
		t.Fatalf("session input subject: %v", err)
	}
	if subject != "session.chat_01.input" {
		t.Fatalf("session input subject = %q", subject)
	}
	artifactSubject, err := ArtifactDataSubject("123-PDF")
	if err != nil {
		t.Fatalf("artifact subject: %v", err)
	}
	if artifactSubject != "artifact.id_123_pdf.data" {
		t.Fatalf("artifact subject = %q", artifactSubject)
	}
}

func TestClientSubjectRejectsEmptySession(t *testing.T) {
	if _, err := SessionEventsSubject(" "); err == nil {
		t.Fatal("expected empty session error")
	}
}
