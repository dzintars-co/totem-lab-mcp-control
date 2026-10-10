#!/usr/bin/env bash
# Build the sketch with the bundled Arduino AVR core. No installation or upload.
# Veido HEX ar komplektā esošo Arduino AVR kodolu; neko neinstalē un neiešuj.
set -euo pipefail

project_root=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
core_root="$project_root/third_party/arduino-avr-core/1.8.8"
sketch_root="$project_root/Projects/totem-mcp/firmware/totem_uno"

# Use an explicit toolchain, an existing Arduino installation, or PATH.
toolchain=${TOTEM_AVR_TOOLCHAIN:-}
if [[ -z "$toolchain" ]]; then
  installed="${HOME:?}/.arduino15/packages/arduino/tools/avr-gcc/7.3.0-atmel3.6.1-arduino7"
  if [[ -x "$installed/bin/avr-g++" ]]; then
    toolchain="$installed"
  fi
fi
compiler_prefix=""
if [[ -n "$toolchain" ]]; then
  compiler_prefix="$toolchain/bin/"
fi
for tool in avr-gcc avr-g++ avr-gcc-ar avr-objcopy avr-size; do
  if ! command -v "${compiler_prefix}${tool}" >/dev/null 2>&1; then
    printf 'Missing %s. Set TOTEM_AVR_TOOLCHAIN to an installed AVR GCC directory.\n' "$tool" >&2
    exit 1
  fi
done

mkdir -p -- "$project_root/.cache"
build_dir=$(mktemp -d "$project_root/.cache/firmware-build.XXXXXX")
mkdir -p -- "$build_dir/core"
defines=(-mmcu=atmega328p -DF_CPU=16000000L -DARDUINO=10607
         -DARDUINO_AVR_UNO -DARDUINO_ARCH_AVR)
includes=("-I$core_root/cores/arduino" "-I$core_root/variants/standard"
          "-I$sketch_root")
c_flags=(-c -g -Os -w -std=gnu11 -ffunction-sections -fdata-sections
         -MMD -flto -fno-fat-lto-objects)
cpp_flags=(-c -g -Os -w -std=gnu++11 -fpermissive -fno-exceptions
           -ffunction-sections -fdata-sections -fno-threadsafe-statics
           -Wno-error=narrowing -MMD -flto)
asm_flags=(-c -g -x assembler-with-cpp -flto -MMD)

"${compiler_prefix}avr-g++" "${cpp_flags[@]}" "${defines[@]}" "${includes[@]}" \
  -x c++ "$sketch_root/totem_uno.ino" -o "$build_dir/sketch.o"

objects=()
for source in "$core_root/cores/arduino/"*.c \
              "$core_root/cores/arduino/"*.cpp \
              "$core_root/cores/arduino/"*.S; do
  [[ -f "$source" ]] || continue
  object="$build_dir/core/$(basename -- "$source").o"
  case "$source" in
    *.cpp) "${compiler_prefix}avr-g++" "${cpp_flags[@]}" "${defines[@]}" \
             "${includes[@]}" "$source" -o "$object" ;;
    *.c)   "${compiler_prefix}avr-gcc" "${c_flags[@]}" "${defines[@]}" \
             "${includes[@]}" "$source" -o "$object" ;;
    *.S)   "${compiler_prefix}avr-gcc" "${asm_flags[@]}" "${defines[@]}" \
             "${includes[@]}" "$source" -o "$object" ;;
  esac
  objects+=("$object")
done
"${compiler_prefix}avr-gcc-ar" rcs "$build_dir/core.a" "${objects[@]}"
"${compiler_prefix}avr-gcc" -w -Os -g -flto -fuse-linker-plugin \
  -Wl,--gc-sections -mmcu=atmega328p -o "$build_dir/totem_uno.elf" \
  "$build_dir/sketch.o" "$build_dir/core.a" "-L$build_dir" -lm
"${compiler_prefix}avr-objcopy" -O ihex -R .eeprom \
  "$build_dir/totem_uno.elf" "$build_dir/totem_uno.ino.hex"
"${compiler_prefix}avr-size" -C --mcu=atmega328p "$build_dir/totem_uno.elf"
printf '\nBuilt HEX: %s\n' "$build_dir/totem_uno.ino.hex"
