package main

import (
	"context"
	"errors"
	"sync"
)

type Result[R any] struct {
	Value R
	Err   error
}

var ErrInvalidWorkers = errors.New("workers must be positive")

func ParallelMap[T any, R any](ctx context.Context, workers int, in []T, fn func(context.Context, T) (R, error)) ([]Result[R], error) {
	if workers <= 0 {
		return nil, ErrInvalidWorkers
	}

	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	if len(in) == 0 {
		return []Result[R]{}, nil
	}

	if workers > len(in) {
		workers = len(in)
	}
	results := make([]Result[R], len(in))
	jobs := make(chan int)
	var wg sync.WaitGroup

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for index := range jobs {
				if ctx.Err() != nil {
					return
				}
				value, err := fn(ctx, in[index])
				if err != nil {
					results[index] = Result[R]{Err: err}
				} else {
					results[index] = Result[R]{Value: value}
				}
			}
		}()
	}

	for i := range in {
		if ctx.Err() != nil {
			break
		}
		select {
			case jobs <- i:
			case <-ctx.Done():
		}

		if ctx.Err() != nil {
			break
		}
	}

	close(jobs)
	wg.Wait()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	return results, nil
}