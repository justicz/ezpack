# ezpack

Minimal, easily auditible, reflect-based golang msgpack codec.

I wrote this because I could not find a golang msgpack library that I felt confident was secure against malicious inputs, particularly with regard to denial of service attacks (high memory use, invalid calls to `make`, etc.).

Design goals:
- Gracefully handle malicious inputs
- Encode canonically (an instance of the same type with the same value should always encode the same way)
- Return an error if decoding a non-canonical input
- Write code that is easy to read and understand
- Don't violate the msgpack spec
- Support 32-bit platforms

Non-goals:
- Performance
- Efficiency in size of encoded data
- Support for lots of types

Misc notes:
- `nil` is not supported. `nil` slices are encoded as length 0 slices.

Maybe one day this project will have real documentation :)
