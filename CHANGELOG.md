# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/), and
this project adheres to [Semantic Versioning](https://semver.org/).

## Unreleased

## v0.2.0 (2026-08-20)

* Changed `Server.Serve` to close server-owned SSE clients and join its HTTP
  serve goroutine on cancellation or listener failure. Graceful shutdown has a
  five-second deadline; forced shutdowns now return an error rather than
  reporting successful cancellation.

## v0.1.1 (2026-8-13)

* Added version.go with `LibVersion` and `LibReleaseDate` package variables.

## v0.1.0 (2026-08-13)

* **Added:** Standalone `htxtd` static website server and reusable `Server`
  package API with request logging, optional server-owned live reload, and
  graceful interruption handling.
