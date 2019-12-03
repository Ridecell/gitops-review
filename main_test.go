package main

import "testing"

type diffOkayTestCase struct {
	diff  string
	okay  bool
	focus bool
}

func TestDiffOkayForAutoApprove(t *testing.T) {
	cases := []diffOkayTestCase{
		{
			diff: "",
			okay: false,
		},
		{
			diff: `
diff --git a/us-dev/noah.yml b/us-dev/noah.yml
index dc82f89..e468d66 100644
--- a/us-dev/noah.yml
+++ b/us-dev/noah.yml
@@ -4,7 +4,7 @@ metadata:
   name: noah-dev
   namespace: summon-dev
 spec:
-  version: 93808-0deed1d-master
+  version: 93808-0deed1d-master-foobar
 ---
 apiVersion: secrets.ridecell.io/v1beta1
 kind: EncryptedSecret
`,
			okay: true,
		},
		{
			diff: `
diff --git a/us-dev/noah.yml b/us-dev/noah.yml
index dc82f89..e468d66 100644
--- a/us-dev/noah.yml
+++ b/us-dev/noah.yml
@@ -4,7 +4,7 @@ metadata:
   name: noah-dev
   namespace: summon-dev
 spec:
-  version: 93808-0deed1d-master
+  versiona: 93808-0deed1d-master-foobar
 ---
 apiVersion: secrets.ridecell.io/v1beta1
 kind: EncryptedSecret
`,
			okay: false,
		},
		{
			diff: `
diff --git a/us-prod/noah.yml b/us-prod/noah.yml
index dc82f89..e468d66 100644
--- a/us-prod/noah.yml
+++ b/us-prod/noah.yml
@@ -4,7 +4,7 @@ metadata:
   name: noah-prod
   namespace: summon-prod
 spec:
-  version: 93808-0deed1d-master
+  version: 93808-0deed1d-master-foobar
 ---
 apiVersion: secrets.ridecell.io/v1beta1
 kind: EncryptedSecret
`,
			okay: false,
		},
		{
			diff: `
diff --git a/us-dev/noah.yml b/us-dev/noah.yml
index dc82f89..e468d66 100644
--- a/us-dev/noah.yml
+++ b/us-dev/noah.yml
@@ -4,7 +4,7 @@ metadata:
   name: noah-dev
   namespace: summon-dev
 spec:
-  version: 93808-0deed1d-master
+  version: 93808-0deed1d-master-foobar
+  config:
+    FOO: bar
 ---
 apiVersion: secrets.ridecell.io/v1beta1
 kind: EncryptedSecret
`,
			okay: false,
		},
		{
			diff: `
diff --git a/us-dev/noah.yml b/us-dev/noah.yml
index dc82f89..e468d66 100644
--- a/us-dev/noah.yml
+++ b/us-dev/noah.yml
@@ -4,7 +4,7 @@ metadata:
   name: noah-dev
   namespace: summon-dev
 spec:
-  version: 93808-0deed1d-master
 ---
 apiVersion: secrets.ridecell.io/v1beta1
 kind: EncryptedSecret
`,
			okay: false,
		},
	}

	useFocus := false
	for _, c := range cases {
		if c.focus {
			useFocus = true
		}
	}

	for _, c := range cases {
		if useFocus && !c.focus {
			continue
		}
		okay, _ := diffOkayForAutoApprove(c.diff)
		if okay != c.okay {
			t.Errorf("diffOkayForAutoApprove failed, got %v want %v:\n%s", okay, c.okay, c.diff)
		}
	}
}
