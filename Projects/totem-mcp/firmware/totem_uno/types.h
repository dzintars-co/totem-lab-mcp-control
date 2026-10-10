#pragma once
#include <stdint.h>

// Keep custom types visible to Arduino's generated function prototypes.
struct Debouncer {
  bool raw;
  bool stable;
  uint32_t changedAt;
};
struct Snapshot {
  uint8_t bits;
  uint16_t pots[3];
  int32_t encoder;
  uint8_t red, green, blue, relay;
};
enum FrameKind : uint8_t { READY_FRAME, STATE_FRAME, ACK_FRAME, ERROR_FRAME, GAP_FRAME };
enum ErrorCode : uint8_t { BAD_COMMAND, BAD_ID, BAD_VALUE, BAD_VERSION, LINE_TOO_LONG, BAD_CHAR };
struct Frame {
  Snapshot state;
  uint32_t seq;
  uint32_t id;  // Also holds LOST for GAP_FRAME.
  uint8_t kind;
  uint8_t error;
};
