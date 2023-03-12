package main

import "testing"

func TestSaveData(t *testing.T) {
	infos := make(map[string][]Info)
	infos["123"] = append(infos["123"], Info{
		Name: "123",
	})
	tests := []struct {
		name string
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			SaveData()
		})
	}
}
