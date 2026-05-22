package servicefunction

import "testing"

func TestSubjectNormalizesProductNames(t *testing.T) {
	subject, err := Subject("build-release", "v1", "CreateDraft")
	if err != nil {
		t.Fatalf("subject: %v", err)
	}
	if subject != "svc.build_release.v1.create_draft" {
		t.Fatalf("subject = %q", subject)
	}
}

func TestSubjectFromFunctionName(t *testing.T) {
	subject, err := SubjectFromFunctionName("indexer_GetContext")
	if err != nil {
		t.Fatalf("subject from function name: %v", err)
	}
	if subject != "svc.indexer.v1.get_context" {
		t.Fatalf("subject = %q", subject)
	}
}

func TestValidateSubjectRejectsNonServiceSubject(t *testing.T) {
	if err := ValidateSubject("session.one.events"); err == nil {
		t.Fatal("expected invalid service subject")
	}
}
