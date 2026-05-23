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

func TestSubjectFromOwnerAndFunctionNameKeepsCompoundOwner(t *testing.T) {
	subject, err := SubjectFromOwnerAndFunctionName("build-release", "build_release_DryRun")
	if err != nil {
		t.Fatalf("subject from owner and function: %v", err)
	}
	if subject != "svc.build_release.v1.dry_run" {
		t.Fatalf("subject = %q", subject)
	}
}

func TestFunctionTokenFromOwnerAndFunctionName(t *testing.T) {
	function, err := FunctionTokenFromOwnerAndFunctionName("indexer", "indexer_GetContext")
	if err != nil {
		t.Fatalf("function token: %v", err)
	}
	if function != "get_context" {
		t.Fatalf("function = %q", function)
	}
}

func TestValidateSubjectRejectsNonServiceSubject(t *testing.T) {
	if err := ValidateSubject("session.one.events"); err == nil {
		t.Fatal("expected invalid service subject")
	}
}
