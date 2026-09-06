package reviewers

import "testing"

func TestInventoryJSXExtensionsPreserveSameFileIdentity(t *testing.T) {
	for _, extension := range []string{"tsx", "jsx", "ts", "js"} {
		t.Run(extension, func(t *testing.T) {
			path := "src/components/StatusCard." + extension
			knowledge := KnowledgeContext{ServiceInventory: "`" + path + "`"}
			if got := duplicatePathFindings(knowledge, []string{path}); len(got) != 0 {
				t.Fatalf("same inventoried file is not a duplicate: %+v", got)
			}
			if got := duplicatePathFindings(knowledge, []string{"src/components/StatusCard-v2." + extension}); len(got) != 1 {
				t.Fatalf("a separate equivalent path must still be detected: %+v", got)
			}
		})
	}
}
