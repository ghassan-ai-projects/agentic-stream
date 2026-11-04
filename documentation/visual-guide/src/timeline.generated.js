export const GENERATED = {
  "engine": "openrouter",
  "model": "elevenlabs/eleven-v4",
  "voice": "george",
  "total": 270.261,
  "chapters": [
    {
      "id": "intro",
      "start": 0,
      "duration": 17.122,
      "audio": 14.622
    },
    {
      "id": "gateway",
      "start": 17.122,
      "duration": 12.478,
      "audio": 9.278
    },
    {
      "id": "log",
      "start": 29.6,
      "duration": 16.515,
      "audio": 14.315
    },
    {
      "id": "engine",
      "start": 46.115,
      "duration": 31.133,
      "audio": 28.933
    },
    {
      "id": "situation",
      "start": 77.247,
      "duration": 30.78,
      "audio": 28.18
    },
    {
      "id": "scheduler",
      "start": 108.027,
      "duration": 33.14,
      "audio": 30.54
    },
    {
      "id": "episode",
      "start": 141.168,
      "duration": 29.802,
      "audio": 26.802
    },
    {
      "id": "validator",
      "start": 170.97,
      "duration": 24.642,
      "audio": 22.042
    },
    {
      "id": "policy",
      "start": 195.611,
      "duration": 22.428,
      "audio": 20.028
    },
    {
      "id": "action",
      "start": 218.04,
      "duration": 30.658,
      "audio": 28.058
    },
    {
      "id": "outro",
      "start": 248.697,
      "duration": 21.563,
      "audio": 17.363
    }
  ],
  "captions": [
    {
      "chapter": "intro",
      "i": 0,
      "start": 1.2,
      "end": 5.533,
      "text": "Agentic Stream turns sensor readings into safe, governed action."
    },
    {
      "chapter": "intro",
      "i": 1,
      "start": 6.283,
      "end": 11.517,
      "text": "Here is one real recorded run, with a simulator playing the sensor and the device."
    },
    {
      "chapter": "intro",
      "i": 2,
      "start": 12.367,
      "end": 15.822,
      "text": "Let's follow it from a warming sensor to a verified effect."
    },
    {
      "chapter": "gateway",
      "i": 0,
      "start": 17.822,
      "end": 19.801,
      "text": "A temperature sensor is warming up."
    },
    {
      "chapter": "gateway",
      "i": 1,
      "start": 21.201,
      "end": 27.1,
      "text": "At the edge, each reading becomes an event, with an identity and the time it really happened."
    },
    {
      "chapter": "log",
      "i": 0,
      "start": 30.3,
      "end": 36.175,
      "text": "Ingress checks each event against its schema, then appends it to a log that is never rewritten."
    },
    {
      "chapter": "log",
      "i": 1,
      "start": 37.575,
      "end": 39.995,
      "text": "Malformed input would be quarantined."
    },
    {
      "chapter": "log",
      "i": 2,
      "start": 41.395,
      "end": 44.615,
      "text": "An event is evidence, never an instruction."
    },
    {
      "chapter": "engine",
      "i": 0,
      "start": 46.815,
      "end": 52.331,
      "text": "The stream engine works in event time: when a reading happened, not when it arrived."
    },
    {
      "chapter": "engine",
      "i": 1,
      "start": 53.281,
      "end": 59.791,
      "text": "A watermark trails the newest reading by fifteen seconds, and marks how far time has safely advanced."
    },
    {
      "chapter": "engine",
      "i": 2,
      "start": 60.741,
      "end": 63.725,
      "text": "A late reading still lands at its true time."
    },
    {
      "chapter": "engine",
      "i": 3,
      "start": 65.125,
      "end": 72.515,
      "text": "A five-minute window slides every thirty seconds, and operators turn it into features: the mean, and the slope."
    },
    {
      "chapter": "engine",
      "i": 4,
      "start": 73.565,
      "end": 75.747,
      "text": "Same input, same features."
    },
    {
      "chapter": "situation",
      "i": 0,
      "start": 77.947,
      "end": 82.726,
      "text": "Features update a Situation: the durable record of one condition."
    },
    {
      "chapter": "situation",
      "i": 1,
      "start": 83.576,
      "end": 87.471,
      "text": "A phase changes only when its rule holds for a minimum time."
    },
    {
      "chapter": "situation",
      "i": 2,
      "start": 88.321,
      "end": 95.998,
      "text": "After a minute, it moved from candidate to watch. Thirty seconds later, to cooling, meaning cooling is needed."
    },
    {
      "chapter": "situation",
      "i": 3,
      "start": 97.248,
      "end": 101.82,
      "text": "Each window step publishes a provisional version, then an on-time one."
    },
    {
      "chapter": "situation",
      "i": 4,
      "start": 102.87,
      "end": 106.127,
      "text": "Twenty-two versions, and none can be edited."
    },
    {
      "chapter": "scheduler",
      "i": 0,
      "start": 108.727,
      "end": 114.871,
      "text": "Reasoning is slow and costly, so a deterministic scheduler decides when it is worth it."
    },
    {
      "chapter": "scheduler",
      "i": 1,
      "start": 115.721,
      "end": 118.38,
      "text": "The four candidate versions fail the trigger."
    },
    {
      "chapter": "scheduler",
      "i": 2,
      "start": 119.73,
      "end": 122.422,
      "text": "Version five passes, and is queued."
    },
    {
      "chapter": "scheduler",
      "i": 3,
      "start": 123.772,
      "end": 126.734,
      "text": "Version seven scores higher, and replaces it."
    },
    {
      "chapter": "scheduler",
      "i": 4,
      "start": 128.134,
      "end": 133.595,
      "text": "The other sixteen bring nothing new, so they are ignored, each with a recorded reason."
    },
    {
      "chapter": "scheduler",
      "i": 5,
      "start": 134.845,
      "end": 139.268,
      "text": "Twenty-two versions, one episode. No model call per event."
    },
    {
      "chapter": "episode",
      "i": 0,
      "start": 141.868,
      "end": 145.929,
      "text": "Admission opens one episode: a bounded reasoning session."
    },
    {
      "chapter": "episode",
      "i": 1,
      "start": 146.779,
      "end": 153.155,
      "text": "It is rebound to the newest snapshot, version twenty-two, and that snapshot never changes."
    },
    {
      "chapter": "episode",
      "i": 2,
      "start": 154.405,
      "end": 158.146,
      "text": "A deadline, a budget, and a fence number keep it bounded."
    },
    {
      "chapter": "episode",
      "i": 3,
      "start": 159.396,
      "end": 163.282,
      "text": "Tamoz, the reasoner, runs in a separate worker process."
    },
    {
      "chapter": "episode",
      "i": 4,
      "start": 164.132,
      "end": 168.67,
      "text": "It reads evidence, and can only propose. It cannot act."
    },
    {
      "chapter": "validator",
      "i": 0,
      "start": 171.67,
      "end": 178.101,
      "text": "Tamoz returns a typed decision: a rising over-temperature, at ninety-six percent confidence."
    },
    {
      "chapter": "validator",
      "i": 1,
      "start": 178.951,
      "end": 182.943,
      "text": "It proposes one intent: set the indicator to alert."
    },
    {
      "chapter": "validator",
      "i": 2,
      "start": 184.193,
      "end": 190.493,
      "text": "The validator accepts only what the spec's catalog allows. An unknown intent would be refused."
    },
    {
      "chapter": "validator",
      "i": 3,
      "start": 191.743,
      "end": 193.711,
      "text": "This one passes every check."
    },
    {
      "chapter": "policy",
      "i": 0,
      "start": 196.311,
      "end": 200.814,
      "text": "Policy checks again, against the current state, right before dispatch."
    },
    {
      "chapter": "policy",
      "i": 1,
      "start": 202.214,
      "end": 206.186,
      "text": "The indicator is low risk, so it is approved automatically."
    },
    {
      "chapter": "policy",
      "i": 2,
      "start": 207.436,
      "end": 211.898,
      "text": "A fan, class R2, would wait for a signed human approval."
    },
    {
      "chapter": "policy",
      "i": 3,
      "start": 212.748,
      "end": 216.34,
      "text": "That path is designed, but was not part of this run."
    },
    {
      "chapter": "action",
      "i": 0,
      "start": 218.74,
      "end": 224.205,
      "text": "The command gets a stable idempotency key, and is written to an outbox before it is sent."
    },
    {
      "chapter": "action",
      "i": 1,
      "start": 225.055,
      "end": 230.26,
      "text": "So a crash cannot lose it, and a retry is recognised as the same request."
    },
    {
      "chapter": "action",
      "i": 2,
      "start": 231.31,
      "end": 235.236,
      "text": "It goes out through the device gateway, and the indicator switches on."
    },
    {
      "chapter": "action",
      "i": 3,
      "start": 236.586,
      "end": 241.732,
      "text": "Three answers come back: a receipt, a result, and a state query."
    },
    {
      "chapter": "action",
      "i": 4,
      "start": 242.982,
      "end": 246.797,
      "text": "Only when the observed state matches is the effect verified."
    },
    {
      "chapter": "outro",
      "i": 0,
      "start": 249.397,
      "end": 256.808,
      "text": "Sixty-one events, twenty-two versions, one episode, one decision, one verified effect."
    },
    {
      "chapter": "outro",
      "i": 1,
      "start": 257.858,
      "end": 262.705,
      "text": "Every step left a durable record, so every outcome can be explained."
    },
    {
      "chapter": "outro",
      "i": 2,
      "start": 263.755,
      "end": 266.761,
      "text": "And new readings keep flowing into the same loop."
    }
  ]
};
