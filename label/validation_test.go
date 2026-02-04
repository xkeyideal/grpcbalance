package label

import (
	"regexp"
	"testing"
)

func Test_charCJK(t *testing.T) {
	re := regexp.MustCompile("^[" + charCJK + "]*$")
	cases := map[string]string{
		"简中": "中日韩自由贸易区",
		"繁中": "中日韓自由貿易區",
		"平假": "にっちゅうかんじゆうぼうえききょうてい",
		"片假": "ニッチュウカンジユウボウエキキョウテイ",
		"韩文": "한중일자유무역지대",
	}
	for key, value := range cases {
		t.Run(key, func(t *testing.T) {
			if !re.MatchString(value) {
				t.Fatal(value)
			}
		})
	}
}
