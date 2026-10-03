// Package admission turns pending scheduler items into admitted, epoch-stamped
// episodes. An item that can never be admitted as it stands (cost refusal, a
// reconsideration while its Situation has a live episode, or a fixture
// executor on a production route) is recorded and skipped so it cannot block
// the queue.
package admission
