// Package app runs one episode on the in-process native executor: it decodes the
// trusted request, enforces every budget in a bounded model/tool loop, turns
// oversized tool results into artifacts, and accepts a Decision only after the
// domain validates it. Providers and tools are ports; it holds no I/O.
package app
