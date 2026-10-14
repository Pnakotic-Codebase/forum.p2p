package seedphrase

import "testing"

func benchPhrase(b *testing.B, mutate func([]string) []string) {
	var seed [SeedSize]byte
	for i := range seed {
		seed[i] = byte(i)
	}
	p := mutate(Encode(seed))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Restore(p)
	}
}

func BenchmarkRestoreValid(b *testing.B) {
	benchPhrase(b, func(p []string) []string { return p })
}

func BenchmarkRestoreFirstWordNotOnList(b *testing.B) {
	benchPhrase(b, func(p []string) []string { p[0] = "zzzzzz"; return p })
}

func BenchmarkRestoreLastWordNotOnList(b *testing.B) {
	benchPhrase(b, func(p []string) []string { p[22] = "zzzzzz"; return p })
}

func BenchmarkRestoreChecksumMismatch(b *testing.B) {
	benchPhrase(b, func(p []string) []string { p[0], p[1] = p[1], p[0]; return p })
}

func BenchmarkRestoreTooFewWords(b *testing.B) {
	benchPhrase(b, func(p []string) []string { return p[:22] })
}
