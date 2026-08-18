# Support

Agentic Stream is an unreleased development project. There is no guaranteed
response time or managed production support service.

## Before opening an issue

Read:

- [documentation](documentation/README.md);
- [limitations](documentation/overview/limitations.md);
- [troubleshooting](documentation/guides/troubleshoot.md);
- [contributing](CONTRIBUTING.md);
- [security policy](SECURITY.md) for vulnerabilities.

## Bug reports

Open a [new issue](https://github.com/ghassan-ai-projects/agentic-stream/issues/new/choose)
for a reproducible, non-sensitive bug. Include:

- commit or version output from `agentic-stream version`;
- operating system and Go version;
- exact command and flags, with secrets removed;
- spec digest or a minimal redacted spec;
- trace shape or a minimal redacted trace;
- database/runtime mode and the observed output/error;
- relevant test or reproduction steps.

Never attach credentials, private keys, capability tokens, raw restricted
payloads, or an unredacted runtime database.

## Questions and proposals

Use a discussion or issue when the question is about supported behavior. For
architecture, contract, storage, worker, policy, or security changes, include
the invariant, source-of-truth files, risks, and proposed evidence.

## Security reports

Do not use a public issue for a vulnerability. Follow [SECURITY.md](SECURITY.md).
