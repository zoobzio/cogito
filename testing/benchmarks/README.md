# Benchmarks

Performance benchmarks for cogito components.

## Running Benchmarks

```bash
# Run all benchmarks
make bench

# Run with more iterations
go test -bench=. -benchmem -benchtime=1s ./testing/benchmarks/...

# Run specific benchmark
go test -bench=BenchmarkThoughtCreation -benchmem ./testing/benchmarks/...

# Generate CPU profile
go test -bench=. -cpuprofile=cpu.prof ./testing/benchmarks/...

# Generate memory profile
go test -bench=. -memprofile=mem.prof ./testing/benchmarks/...
```

## Benchmarks

| Benchmark | Description |
|-----------|-------------|
| `BenchmarkThoughtCreation` | Measures thought creation overhead |
| `BenchmarkNoteAccumulation` | Measures note addition performance |
| `BenchmarkThoughtClone` | Measures thought cloning for parallel ops |

## Interpreting Results

```
BenchmarkThoughtCreation-8    50000    25000 ns/op    4096 B/op    50 allocs/op
```

- `50000` - Number of iterations
- `25000 ns/op` - Nanoseconds per operation
- `4096 B/op` - Bytes allocated per operation
- `50 allocs/op` - Heap allocations per operation

## Performance Considerations

1. **Thought Creation**: Initial thought creation includes database persistence
2. **Note Accumulation**: Notes are stored in append-only slice with map index
3. **Cloning**: Clone performs deep copy of all notes and session state
