package internal

import "iter"

func ConcatSeq2[K, V any](sequences ...iter.Seq2[K, V]) iter.Seq2[K, V] {
	return func(yield func(K, V) bool) {
		for _, seq := range sequences {
			for k, v := range seq {
				if !yield(k, v) {
					return
				}
			}
		}
	}
}
