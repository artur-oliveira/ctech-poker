package userpurge

import (
	"os"
	"regexp"
	"testing"
)

// Every poker_* table the CDK declares must have a decided treatment here.
// A new table that holds player data and is not added to Purge is a silent
// LGPD gap; this makes it a red build instead.
func TestInventoryClassifiesEveryTable(t *testing.T) {
	re := regexp.MustCompile(`'(poker_[a-z_]+)'`)
	for _, file := range []string{"../../../cdk/lib/dynamodb-stack.ts", "../../../cdk/lib/constants.ts"} {
		src, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		for _, m := range re.FindAllStringSubmatch(string(src), -1) {
			if _, ok := inventory[m[1]]; !ok {
				t.Errorf("%s declares %s, which userpurge does not classify: decide erase/anonymize/retain (data inventory §7) and handle it in Purge", file, m[1])
			}
		}
	}
}
