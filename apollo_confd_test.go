package main

import "testing"

func TestSplitAndJoinKey(t *testing.T) {
	testCases := []struct {
		File       string
		Ns         string
		Key        string
		JoinedFile string
	}{
		{
			File:       "ns1/testkey",
			Ns:         "ns1",
			Key:        "testkey",
			JoinedFile: "ns1/testkey",
		},
		{
			File:       "ns1/",
			Ns:         "ns1",
			Key:        "default",
			JoinedFile: "ns1/default",
		},
		{
			File:       "/testkey",
			Ns:         "default",
			Key:        "testkey",
			JoinedFile: "default/testkey",
		},
		{
			File:       "testkey",
			Ns:         "default",
			Key:        "testkey",
			JoinedFile: "default/testkey",
		},
	}
	for _, c := range testCases {
		{
			ns, key := splitKey(c.File)
			if ns != c.Ns {
				t.Logf("case %s expect namespace %s, but got %s", c.File, c.Ns, ns)
				t.Fail()
			}
			if key != c.Key {
				t.Logf("case %s expect key %s, but got %s", c.File, c.Key, key)
				t.Fail()
			}
		}
		{
			key := joinKey(c.Ns, c.Key)
			if key != c.JoinedFile {
				t.Logf("case namespace %s, key %s, expect %s, but got %s", c.Ns, c.Key, c.JoinedFile, key)
			}
		}
	}
}
