// TotemDuino v1.9 (Uno / ATmega328P), I/O Side Panel v1.7.
// No third-party Arduino libraries. Pin allocation is listed in the project README.
#include <Arduino.h>
#include <avr/pgmspace.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include "types.h"

// Project defaults, not manufacturer-prescribed timings.
const uint32_t BAUD_RATE = 115200;
const uint16_t DEBOUNCE_MS = 15;
const uint16_t POT_SAMPLE_MS = 20;
const uint16_t POT_DELTA = 4;
const uint16_t ENCODER_REPORT_MS = 10;
const uint16_t LED_BLINK_MS = 40;
const uint16_t LED_ACTIVITY_MS = 120;
const int8_t ENCODER_DIRECTION = 1;  // Change to -1 if the physical direction is opposite.

const uint8_t INPUT_PINS[7] = {7, 8, 10, 11, 12, A3, 4};
const uint8_t POT_PINS[3] = {A0, A1, A2};
const uint8_t RGB_R = 5, RGB_G = 6, RGB_B = 9;
const uint8_t RELAY_PIN = A4, ACTIVITY_PIN = 13;
const uint8_t ENCODER_A = 2, ENCODER_B = 3;

void stateFrame(uint8_t kind, uint32_t id = 0);

const uint8_t QUEUE_SIZE = 16;
Frame queue[QUEUE_SIZE];
uint8_t queueHead = 0, queueCount = 0;
Debouncer inputs[7];
Snapshot current = {};
volatile uint32_t encoderTicks = 0;
volatile uint8_t encoderPhase = 0;
uint32_t sequence = 0, lostFrames = 0;
uint32_t lastPotSample = 0, lastEncoderReport = 0;
char receiveBuffer[80];
uint8_t receiveLength = 0;
bool discardLine = false;
uint8_t discardError = LINE_TOO_LONG;
char transmitBuffer[240];
uint16_t transmitLength = 0, transmitPosition = 0;
uint32_t lastActivity = 0, lastLEDToggle = 0;
bool trafficSeen = false, ledOn = false;

void encoderInterrupt() {
  // Each valid quadrature edge is one count; not an assumed mechanical click.
  static const int8_t transitions[16] = {0, -1, 1, 0, 1, 0, 0, -1, -1, 0, 0, 1, 0, 1, -1, 0};
  const uint8_t phase = (digitalRead(ENCODER_A) << 1) | digitalRead(ENCODER_B);
  const int8_t delta = transitions[(encoderPhase << 2) | phase] * ENCODER_DIRECTION;
  encoderTicks += delta;  // Unsigned arithmetic gives defined counter wraparound.
  encoderPhase = phase;
}

int32_t readEncoder() {
  noInterrupts();
  const uint32_t value = encoderTicks;
  interrupts();
  return static_cast<int32_t>(value);
}

void activity() {
  const uint32_t now = millis();
  if (!trafficSeen || static_cast<uint32_t>(now - lastActivity) >= LED_ACTIVITY_MS) {
    ledOn = true;
    digitalWrite(ACTIVITY_PIN, HIGH);
    lastLEDToggle = now;
  }
  trafficSeen = true;
  lastActivity = now;
}

void updateLED() {
  const uint32_t now = millis();
  if (!trafficSeen || static_cast<uint32_t>(now - lastActivity) >= LED_ACTIVITY_MS) {
    ledOn = false;
    digitalWrite(ACTIVITY_PIN, LOW);
  } else if (static_cast<uint32_t>(now - lastLEDToggle) >= LED_BLINK_MS) {
    ledOn = !ledOn;
    digitalWrite(ACTIVITY_PIN, ledOn ? HIGH : LOW);
    lastLEDToggle = now;
  }
}

void applyOutputs() {
  analogWrite(RGB_R, 255 - current.red);
  analogWrite(RGB_G, 255 - current.green);
  analogWrite(RGB_B, 255 - current.blue);
  digitalWrite(RELAY_PIN, current.relay ? HIGH : LOW);
}

bool enqueue(const Frame &frame) {
  if (queueCount == QUEUE_SIZE) return false;
  queue[(queueHead + queueCount) % QUEUE_SIZE] = frame;
  ++queueCount;
  return true;
}

void stateFrame(uint8_t kind, uint32_t id) {
  Frame frame = {};
  frame.kind = kind;
  frame.id = id;
  frame.seq = ++sequence;
  frame.state = current;
  if (!enqueue(frame) && lostFrames != UINT32_MAX) ++lostFrames;
}

void errorFrame(uint32_t id, uint8_t code) {
  Frame frame = {};
  frame.kind = ERROR_FRAME;
  frame.id = id;
  frame.error = code;
  enqueue(frame); // pumpReceive only processes commands while a queue slot is free.
}

bool parseID(const char *text, uint32_t &value) {
  if (!text || !*text) return false;
  value = 0;
  while (*text) {
    if (*text < '0' || *text > '9') return false;
    const uint8_t digit = *text++ - '0';
    if (value > (UINT32_MAX - digit) / 10) return false;
    value = value * 10 + digit;
  }
  return value != 0;
}

int8_t hexDigit(char c) {
  if (c >= '0' && c <= '9') return c - '0';
  if (c >= 'a' && c <= 'f') return c - 'a' + 10;
  if (c >= 'A' && c <= 'F') return c - 'A' + 10;
  return -1;
}

void command(char *line) {
  // Exact order: KIND;V=1;ID=n[;VALUE=x]. Empty and extra fields are rejected.
  char *tokens[5] = {};
  uint8_t count = 0;
  tokens[count++] = line;
  for (char *p = line; *p; ++p) {
    if (*p == ';') {
      *p = '\0';
      if (count == 5) { errorFrame(0, BAD_COMMAND); return; }
      tokens[count++] = p + 1;
    }
  }
  uint32_t id = 0;
  if (count < 3 || strncmp(tokens[2], "ID=", 3) || !parseID(tokens[2] + 3, id)) {
    errorFrame(0, BAD_ID); return;
  }
  if (strcmp(tokens[1], "V=1")) { errorFrame(id, BAD_VERSION); return; }
  if (!strcmp(tokens[0], "GET") && count == 3) {
    current.encoder = readEncoder();
    stateFrame(ACK_FRAME, id);
    return;
  }
  if (count != 4 || strncmp(tokens[3], "VALUE=", 6)) { errorFrame(id, BAD_COMMAND); return; }
  const char *value = tokens[3] + 6;
  if (!strcmp(tokens[0], "RGB")) {
    if (strlen(value) != 7 || value[0] != '#') { errorFrame(id, BAD_VALUE); return; }
    uint8_t channels[3];
    for (uint8_t i = 0; i < 3; ++i) {
      const int8_t hi = hexDigit(value[1 + 2 * i]), lo = hexDigit(value[2 + 2 * i]);
      if (hi < 0 || lo < 0) { errorFrame(id, BAD_VALUE); return; }
      channels[i] = (hi << 4) | lo;
    }
    current.red = channels[0]; current.green = channels[1]; current.blue = channels[2];
  } else if (!strcmp(tokens[0], "RELAY")) {
    if (strcmp(value, "0") && strcmp(value, "1")) { errorFrame(id, BAD_VALUE); return; }
    current.relay = value[0] - '0';
  } else { errorFrame(id, BAD_COMMAND); return; }
  applyOutputs();
  current.encoder = readEncoder();
  stateFrame(ACK_FRAME, id);  // Applied software values, not optical/contact feedback.
}

void pumpReceive() {
  while (Serial.available() && queueCount < QUEUE_SIZE) {
    const char c = Serial.read();
    activity();
    if (c == '\r') continue;
    if (c == '\n') {
      if (discardLine) errorFrame(0, discardError);
      else if (receiveLength) { receiveBuffer[receiveLength] = '\0'; command(receiveBuffer); }
      receiveLength = 0; discardLine = false;
    } else if (!discardLine) {
      if (c < 32 || c > 126) { discardLine = true; discardError = BAD_CHAR; }
      else if (receiveLength == sizeof(receiveBuffer) - 1) { discardLine = true; discardError = LINE_TOO_LONG; }
      else receiveBuffer[receiveLength++] = c;
    }
  }
}

void formatFrame(const Frame &frame) {
  int n = 0;
  if (frame.kind == READY_FRAME) {
    n = snprintf_P(transmitBuffer, sizeof(transmitBuffer), PSTR("READY;V=1\n"));
  } else if (frame.kind == GAP_FRAME) {
    n = snprintf_P(transmitBuffer, sizeof(transmitBuffer), PSTR("GAP;V=1;LOST=%lu\n"), static_cast<unsigned long>(frame.id));
  } else if (frame.kind == ERROR_FRAME) {
    const char *codes[] = {"BAD_COMMAND", "BAD_ID", "BAD_VALUE", "BAD_VERSION", "LINE_TOO_LONG", "BAD_CHAR"};
    n = snprintf_P(transmitBuffer, sizeof(transmitBuffer), PSTR("ERR;V=1;ID=%lu;CODE=%s\n"), static_cast<unsigned long>(frame.id), codes[frame.error]);
  } else {
    const Snapshot &s = frame.state;
    const bool ack = frame.kind == ACK_FRAME;
    n = snprintf_P(transmitBuffer, sizeof(transmitBuffer),
        PSTR("%s;V=1;SEQ=%lu;SW1=%u;SW2=%u;SW3=%u;B1=%u;B2=%u;B3=%u;P1=%u;P2=%u;P3=%u;ENC=%ld;EB=%u;RGB=#%02X%02X%02X;RELAY=%u"),
        ack ? "ACK" : "STATE", static_cast<unsigned long>(frame.seq),
        (s.bits >> 0) & 1, (s.bits >> 1) & 1, (s.bits >> 2) & 1,
        (s.bits >> 3) & 1, (s.bits >> 4) & 1, (s.bits >> 5) & 1,
        s.pots[0], s.pots[1], s.pots[2], static_cast<long>(s.encoder), (s.bits >> 6) & 1,
        s.red, s.green, s.blue, s.relay);
    if (n > 0 && n < static_cast<int>(sizeof(transmitBuffer))) {
      if (ack) n += snprintf_P(transmitBuffer + n, sizeof(transmitBuffer) - n, PSTR(";ID=%lu\n"), static_cast<unsigned long>(frame.id));
      else n += snprintf_P(transmitBuffer + n, sizeof(transmitBuffer) - n, PSTR("\n"));
    }
  }
  transmitLength = n > 0 && n < static_cast<int>(sizeof(transmitBuffer)) ? n : 0;
  transmitPosition = 0;
}

void pumpTransmit() {
  if (transmitPosition == transmitLength && queueCount) {
    formatFrame(queue[queueHead]);
    queueHead = (queueHead + 1) % QUEUE_SIZE;
    --queueCount;
  }
  // Never wait for a complete line to fit in the UART buffer.
  while (transmitPosition < transmitLength && Serial.availableForWrite() > 0) {
    Serial.write(static_cast<uint8_t>(transmitBuffer[transmitPosition++]));
    activity();
  }
}

void scanInputs() {
  const uint32_t now = millis();
  bool changed = false;
  for (uint8_t i = 0; i < 7; ++i) {
    const bool pressed = digitalRead(INPUT_PINS[i]) == LOW;
    if (pressed != inputs[i].raw) { inputs[i].raw = pressed; inputs[i].changedAt = now; }
    if (pressed != inputs[i].stable && static_cast<uint32_t>(now - inputs[i].changedAt) >= DEBOUNCE_MS) {
      inputs[i].stable = pressed;
      if (pressed) current.bits |= 1 << i; else current.bits &= ~(1 << i);
      changed = true;
    }
  }
  if (static_cast<uint32_t>(now - lastPotSample) >= POT_SAMPLE_MS) {
    lastPotSample = now;
    for (uint8_t i = 0; i < 3; ++i) {
      const uint16_t sample = analogRead(POT_PINS[i]);
      const int difference = static_cast<int>(sample) - static_cast<int>(current.pots[i]);
      if (abs(difference) >= POT_DELTA || ((sample == 0 || sample == 1023) && sample != current.pots[i])) {
        current.pots[i] = sample; changed = true;
      }
    }
  }
  if (static_cast<uint32_t>(now - lastEncoderReport) >= ENCODER_REPORT_MS) {
    lastEncoderReport = now;
    const int32_t value = readEncoder();
    if (value != current.encoder) { current.encoder = value; changed = true; }
  }
  if (changed) stateFrame(STATE_FRAME);
}

void setup() {
  // Preload inactive levels before switching pins to OUTPUT.
  digitalWrite(RELAY_PIN, LOW); pinMode(RELAY_PIN, OUTPUT);
  digitalWrite(RGB_R, HIGH); digitalWrite(RGB_G, HIGH); digitalWrite(RGB_B, HIGH);
  pinMode(RGB_R, OUTPUT); pinMode(RGB_G, OUTPUT); pinMode(RGB_B, OUTPUT);
  digitalWrite(ACTIVITY_PIN, LOW); pinMode(ACTIVITY_PIN, OUTPUT);
  for (uint8_t i = 0; i < 7; ++i) {
    pinMode(INPUT_PINS[i], i == 6 ? INPUT : INPUT_PULLUP);
    inputs[i].raw = inputs[i].stable = digitalRead(INPUT_PINS[i]) == LOW;
    if (inputs[i].stable) current.bits |= 1 << i;
  }
  for (uint8_t i = 0; i < 3; ++i) current.pots[i] = analogRead(POT_PINS[i]);
  pinMode(ENCODER_A, INPUT); pinMode(ENCODER_B, INPUT);
  encoderPhase = (digitalRead(ENCODER_A) << 1) | digitalRead(ENCODER_B);
  attachInterrupt(digitalPinToInterrupt(ENCODER_A), encoderInterrupt, CHANGE);
  attachInterrupt(digitalPinToInterrupt(ENCODER_B), encoderInterrupt, CHANGE);
  applyOutputs();
  Serial.begin(BAUD_RATE);
  Frame ready = {}; ready.kind = READY_FRAME; enqueue(ready);
  stateFrame(STATE_FRAME);
}

void loop() {
  pumpTransmit();
  scanInputs();
  // Bounded queues cannot promise unlimited retention: announce loss and resynchronize.
  if (lostFrames && queueCount <= QUEUE_SIZE - 2) {
    Frame gap = {}; gap.kind = GAP_FRAME; gap.id = lostFrames;
    lostFrames = 0; enqueue(gap); stateFrame(STATE_FRAME);
  }
  pumpReceive();
  pumpTransmit();
  updateLED();
}
