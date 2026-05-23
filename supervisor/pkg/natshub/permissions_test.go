package natshub

import "testing"

func TestSpaceAccountNameIsStableAndSanitized(t *testing.T) {
	account, err := SpaceAccountName("Docs.Space-01")
	if err != nil {
		t.Fatalf("space account name: %v", err)
	}
	if account != "SPACE_DOCS_SPACE_01" {
		t.Fatalf("account = %q", account)
	}
}

func TestSessionPermissionsAreScopedToOneSession(t *testing.T) {
	perms := SessionPermissions("session-01")
	assertContains(t, perms.PublishAllow, "session.session_01.input")
	assertContains(t, perms.SubscribeAllow, "session.session_01.events")
	assertContains(t, perms.SubscribeAllow, "session.session_01.status")
	assertNotContains(t, perms.SubscribeAllow, "session.other.events")
}

func TestRuntimePermissionsCanRequestImportedServiceFunctions(t *testing.T) {
	perms := RuntimePermissions()
	assertContains(t, perms.PublishAllow, "catalog.runtime.v1.get")
	assertContains(t, perms.PublishAllow, "svc.>")
	assertContains(t, perms.PublishAllow, "runtime.activity.v1.events")
	assertContains(t, perms.SubscribeAllow, "catalog.runtime.v1.events")
	assertContains(t, perms.SubscribeAllow, "runtime.info.v1.get")
	assertContains(t, perms.SubscribeAllow, "runtime.session.v1.get")
	assertContains(t, perms.SubscribeAllow, "runtime.plan.v1.*")
	assertContains(t, perms.SubscribeAllow, "runtime.activity.v1.list")
	assertContains(t, perms.SubscribeAllow, "_INBOX.>")
}

func TestUserPermissionsCanReachRuntimeInspectionOnly(t *testing.T) {
	perms := UserPermissions()
	assertContains(t, perms.PublishAllow, "catalog.runtime.v1.get")
	assertContains(t, perms.PublishAllow, "runtime.info.v1.get")
	assertContains(t, perms.PublishAllow, "runtime.session.v1.get")
	assertContains(t, perms.PublishAllow, "runtime.plan.v1.get")
	assertContains(t, perms.PublishAllow, "runtime.activity.v1.list")
	assertContains(t, perms.SubscribeAllow, "catalog.runtime.v1.events")
	assertContains(t, perms.SubscribeAllow, "runtime.activity.v1.events")
	assertNotContains(t, perms.PublishAllow, "session.session_01.input")
	assertNotContains(t, perms.SubscribeAllow, "session.session_01.events")
}

func assertContains(t *testing.T, values []string, want string) {
	t.Helper()
	for _, value := range values {
		if value == want {
			return
		}
	}
	t.Fatalf("%q not found in %#v", want, values)
}

func assertNotContains(t *testing.T, values []string, want string) {
	t.Helper()
	for _, value := range values {
		if value == want {
			t.Fatalf("%q unexpectedly found in %#v", want, values)
		}
	}
}
