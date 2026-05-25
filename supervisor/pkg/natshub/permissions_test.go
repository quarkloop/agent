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

func TestSessionPermissionsUseClientSubjectTokenForNumericIDs(t *testing.T) {
	perms := SessionPermissions("70d6e473e286ec0d")
	assertContains(t, perms.PublishAllow, "session.id_70d6e473e286ec0d.input")
	assertContains(t, perms.SubscribeAllow, "session.id_70d6e473e286ec0d.events")
	assertContains(t, perms.SubscribeAllow, "session.id_70d6e473e286ec0d.status")
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

func TestRunStateStoragePermissionsAreNotGrantedToOtherServices(t *testing.T) {
	route := []ServiceFunctionRoute{{ExportSubject: "svc.runstate.v1.acquire_lease"}}
	runstate, err := serviceCredential("runstate", ControlAccountName, route)
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, runstate.Permissions.PublishAllow, "$JS.API.>")
	assertContains(t, runstate.Permissions.PublishAllow, "$KV.>")
	assertContains(t, runstate.Permissions.SubscribeAllow, "_INBOX.>")

	indexer, err := serviceCredential("indexer", ControlAccountName, []ServiceFunctionRoute{{ExportSubject: "svc.indexer.v1.index"}})
	if err != nil {
		t.Fatal(err)
	}
	assertNotContains(t, indexer.Permissions.PublishAllow, "$JS.API.>")
	assertNotContains(t, indexer.Permissions.PublishAllow, "$KV.>")
	assertNotContains(t, indexer.Permissions.SubscribeAllow, "_INBOX.>")
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
