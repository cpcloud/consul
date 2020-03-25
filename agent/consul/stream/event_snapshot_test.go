package stream

import (
	"context"
	fmt "fmt"
	"testing"
	time "time"

	"github.com/hashicorp/consul/agent/agentpb"
	"github.com/stretchr/testify/require"
)

func TestEventSnapshot(t *testing.T) {
	// Setup a dummy state that we can manipulate easily. The properties we care
	// about are that we publish some sequence of events as a snapshot and then
	// follow them up with "live updates". We control the interleavings. Our state
	// consists of health events (only type fully defined so far) for service
	// instances with consecutive ID numbers starting from 0 (e.g. test-000,
	// test-001). The snapshot is delivered at index 1000. updatesBeforeSnap
	// controls how many updates are delivered _before_ the snapshot is complete
	// (with an index < 1000). updatesBeforeSnap controls the number of updates
	// delivered after (index > 1000).
	//
	// In all cases the invariant should be that we end up with all of the
	// instances in the snapshot, plus any delivered _after_ the snapshot index,
	// but none delivered _before_ the snapshot index otherwise we may have an
	// inconsistent snapshot.
	cases := []struct {
		name              string
		snapshotSize      int
		updatesBeforeSnap int
		updatesAfterSnap  int
	}{
		{
			name:              "snapshot with subsequent mutations",
			snapshotSize:      10,
			updatesBeforeSnap: 0,
			updatesAfterSnap:  10,
		},
		{
			name:              "snapshot with concurrent mutations",
			snapshotSize:      10,
			updatesBeforeSnap: 5,
			updatesAfterSnap:  5,
		},
		{
			name:              "empty snapshot with subsequent mutations",
			snapshotSize:      0,
			updatesBeforeSnap: 0,
			updatesAfterSnap:  10,
		},
		{
			name:              "empty snapshot with concurrent mutations",
			snapshotSize:      0,
			updatesBeforeSnap: 5,
			updatesAfterSnap:  5,
		},
	}

	snapIndex := uint64(1000)

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			require.True(t, tc.updatesBeforeSnap < 999,
				"bad test param updatesBeforeSnap must be less than the snapshot"+
					" index (%d) minus one (%d), got: %d", snapIndex, snapIndex-1,
				tc.updatesBeforeSnap)

			// Create a snapshot func that will deliver registration events.
			snFn := testHealthConsecutiveSnapshotFn(tc.snapshotSize, snapIndex)

			// Create a topic buffer for updates
			tb := NewEventBuffer()

			// Capture the topic buffer head now so updatesBeforeSnap are "concurrent"
			// and are seen by the EventSnapshot once it completes the snap.
			tbHead := tb.Head()

			// Deliver any pre-snapshot events simulating updates that occur after the
			// topic buffer is captured during a Subscribe call, but before the
			// snapshot is made of the FSM.
			for i := tc.updatesBeforeSnap; i > 0; i-- {
				index := snapIndex - uint64(i)
				// Use an instance index that's unique and should never appear in the
				// output so we can be sure these were not included as they came before
				// the snapshot.
				tb.Append([]agentpb.Event{testHealthEvent(index, 10000+i)})
			}

			// Create EventSnapshot, (will call snFn in another goroutine). The
			// Request is ignored by the SnapFn so doesn't matter for now.
			es := NewEventSnapshot(&agentpb.SubscribeRequest{}, tbHead, snFn)

			// Deliver any post-snapshot events simulating updates that occur
			// logically after snapshot. It doesn't matter that these might actually
			// be appended before the snapshot fn executes in another goroutine since
			// it's operating an a possible stale "snapshot". This is the same as
			// reality with the state store where updates that occur after the
			// snapshot is taken but while the SnapFnis still running must be captured
			// correctly.
			for i := 0; i < tc.updatesAfterSnap; i++ {
				index := snapIndex + 1 + uint64(i)
				// Use an instance index that's unique.
				tb.Append([]agentpb.Event{testHealthEvent(index, 20000+i)})
			}

			// Now read the snapshot buffer until we've received everything we expect.
			// Don't wait too long in case we get stuck.
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			snapIDs := make([]string, 0, tc.snapshotSize)
			updateIDs := make([]string, 0, tc.updatesAfterSnap)
			snapDone := false
			curItem := es.Snap
			var err error
		RECV:
			for {
				curItem, err = curItem.Next(ctx)
				// This error is typically timeout so dump the state to aid debugging.
				require.NoError(t, err,
					"current state: snapDone=%v snapIDs=%s updateIDs=%s", snapDone,
					snapIDs, updateIDs)
				e := curItem.Events[0]
				if snapDone {
					sh := e.GetServiceHealth()
					require.NotNil(t, sh, "want health event got: %#v", e.Payload)
					updateIDs = append(updateIDs, sh.CheckServiceNode.Service.ID)
					if len(updateIDs) == tc.updatesAfterSnap {
						// We're done!
						break RECV
					}
				} else if e.GetEndOfSnapshot() {
					snapDone = true
				} else {
					sh := e.GetServiceHealth()
					require.NotNil(t, sh, "want health event got: %#v", e.Payload)
					snapIDs = append(snapIDs, sh.CheckServiceNode.Service.ID)
				}
			}

			// Validate the event IDs we got delivered.
			require.Equal(t, genSequentialIDs(0, tc.snapshotSize), snapIDs)
			require.Equal(t, genSequentialIDs(20000, 20000+tc.updatesAfterSnap), updateIDs)
		})
	}
}

func genSequentialIDs(start, end int) []string {
	ids := make([]string, 0, end-start)
	for i := start; i < end; i++ {
		ids = append(ids, fmt.Sprintf("test-%03d", i))
	}
	return ids
}

func testHealthConsecutiveSnapshotFn(size int, index uint64) SnapFn {
	return func(req *agentpb.SubscribeRequest, buf *EventBuffer) (uint64, error) {
		for i := 0; i < size; i++ {
			// Event content is arbitrary we are just using Health because it's the
			// first type defined. We just want a set of things with consecutive
			// names.
			buf.Append([]agentpb.Event{testHealthEvent(index, i)})
		}
		return index, nil
	}
}

func testHealthEvent(index uint64, n int) agentpb.Event {
	return agentpb.Event{
		Index: index,
		Topic: agentpb.Topic_ServiceHealth,
		Payload: &agentpb.Event_ServiceHealth{
			ServiceHealth: &agentpb.ServiceHealthUpdate{
				Op: agentpb.CatalogOp_Register,
				CheckServiceNode: &agentpb.CheckServiceNode{
					Node: &agentpb.Node{
						Node:    "n1",
						Address: "10.10.10.10",
					},
					Service: &agentpb.NodeService{
						ID:      fmt.Sprintf("test-%03d", n),
						Service: "test",
						Port:    8080,
					},
				},
			},
		},
	}
}