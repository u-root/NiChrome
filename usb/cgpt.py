#!/usr/bin/env python2
# Copyright (c) 2012 The Chromium OS Authors. All rights reserved.
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

"""Parse and operate based on disk layout files."""

from __future__ import print_function

import argparse
import copy
import json
import math
import os
import re
import sys


class ConfigNotFound(Exception):
  """Config Not Found"""

class PartitionNotFound(Exception):
  """Partition Not Found"""

class InvalidLayout(Exception):
  """Invalid Layout"""

class InvalidAdjustment(Exception):
  """Invalid Adjustment"""

class InvalidSize(Exception):
  """Invalid Size"""

class ConflictingOptions(Exception):
  """Conflicting Options"""

class ConflictingPartitionOrder(Exception):
  """The partition order in the parent and child layout don't match."""

class MismatchedRootfsFormat(Exception):
  """Rootfs partitions in different formats"""

class MismatchedRootfsBlocks(Exception):
  """Rootfs partitions have different numbers of reserved erase blocks"""

class MissingEraseBlockField(Exception):
  """Partition has reserved erase blocks but not other fields needed"""

class ExcessFailureProbability(Exception):
  """Chances are high that the partition will have too many bad blocks"""

class UnalignedPartition(Exception):
  """Partition size does not divide erase block size"""

class ExpandNandImpossible(Exception):
  """Partition is raw NAND and marked with the incompatible expand feature"""

class ExcessPartitionSize(Exception):
  """Partitions sum to more than the size of the whole device"""

COMMON_LAYOUT = 'common'
BASE_LAYOUT = 'base'
# Blocks of the partition entry array.
SIZE_OF_PARTITION_ENTRY_ARRAY = 32
SIZE_OF_PMBR = 1
SIZE_OF_GPT_HEADER = 1


def ParseHumanNumber(operand):
  """Parse a human friendly number

  This handles things like 4GiB and 4MB and such.  See the usage string for
  full details on all the formats supported.

  Args:
    operand: The number to parse (may be an int or string)

  Returns:
    An integer
  """
  operand = str(operand)
  negative = -1 if operand.startswith('-') else 1
  if negative == -1:
    operand = operand[1:]
  operand_digits = re.sub(r'\D', r'', operand)

  size_factor = block_factor = 1
  suffix = operand[len(operand_digits):].strip()
  if suffix:
    size_factors = {'B': 0, 'K': 1, 'M': 2, 'G': 3, 'T': 4,}
    try:
      size_factor = size_factors[suffix[0].upper()]
    except KeyError:
      raise InvalidAdjustment('Unknown size type %s' % suffix)
    if size_factor == 0 and len(suffix) > 1:
      raise InvalidAdjustment('Unknown size type %s' % suffix)
    block_factors = {'': 1024, 'B': 1000, 'IB': 1024,}
    try:
      block_factor = block_factors[suffix[1:].upper()]
    except KeyError:
      raise InvalidAdjustment('Unknown size type %s' % suffix)

  return int(operand_digits) * pow(block_factor, size_factor) * negative


def ProduceHumanNumber(number):
  """A simple reverse of ParseHumanNumber, converting a number to human form.

  Args:
    number: A number (int) to be converted to human form.

  Returns:
    A string, such as "1 KiB", that satisfies the condition
      ParseHumanNumber(ProduceHumanNumber(i)) == i.
  """
  scales = [
      (2**40, 'Ti'),
      (10**12, 'T'),
      (2**30, 'Gi'),
      (10**9, 'G'),
      (2**20, 'Mi'),
      (10**6, 'M'),
      (2**10, 'Ki'),
      (10**3, 'K')
  ]
  for denom, suffix in scales:
    if (number % denom) == 0:
      return '%d %sB' % (number // denom, suffix)
  return str(number)


def ParseRelativeNumber(max_number, number):
  """Return the number that is relative to |max_number| by |number|

  We support three forms:
   90% - |number| is a percentage of |max_number|
   100 - |number| is the answer already (and |max_number| is ignored)
   -90 - |number| is subtracted from |max_number|

  Args:
    max_number: The limit to use when |number| is negative or a percent
    number: The (possibly relative) number to parse (may be an int or string)
  """
  max_number = int(max_number)
  number = str(number)
  if number.endswith('%'):
    percent = float(number[:-1]) / 100
    return int(max_number * percent)
  else:
    number = ParseHumanNumber(number)
    if number < 0:
      return max_number + number
    else:
      return number


def _ApplyLayoutOverrides(layout_to_override, layout):
  """Applies |layout| overrides on to |layout_to_override|.

  First add missing partition from layout to layout_to_override.
  Then, update partitions in layout_to_override with layout information.
  """
  # First check that all the partitions defined in both layouts are defined in
  # the same order in each layout. Otherwise, the order in which they end up
  # in the merged layout doesn't match what the user sees in the child layout.
  common_nums = set.intersection(
      {part['num'] for part in layout_to_override if 'num' in part},
      {part['num'] for part in layout if 'num' in part})
  layout_to_override_order = [part['num'] for part in layout_to_override
                              if part.get('num') in common_nums]
  layout_order = [part['num'] for part in layout
                  if part.get('num') in common_nums]
  if layout_order != layout_to_override_order:
    raise ConflictingPartitionOrder(
        'Layouts share partitions %s but they are in different order: '
        'layout_to_override: %s, layout: %s' % (
            sorted(common_nums),
            [part.get('num') for part in layout_to_override],
            [part.get('num') for part in layout]))

  # Merge layouts with the partitions in the same order they are in both
  # layouts.
  part_index = 0
  for part_to_apply in layout:
    num = part_to_apply.get('num')

    if part_index == len(layout_to_override):
      # The part_to_apply is past the list of partitions to override, this
      # means that is a new partition added at the end.
      # Need of deepcopy, in case we change layout later.
      layout_to_override.append(copy.deepcopy(part_to_apply))
    elif layout_to_override[part_index].get('num') is None and num is None:
      # Allow modifying gaps after a partition.
      # TODO(deymo): Drop support for "gap" partitions and use alignment
      # instead.
      layout_to_override[part_index].update(part_to_apply)
    elif num in common_nums:
      while layout_to_override[part_index].get('num') != num:
        part_index += 1
      layout_to_override[part_index].update(part_to_apply)
    else:
      # Need of deepcopy, in case we change layout later.
      layout_to_override.insert(part_index, copy.deepcopy(part_to_apply))
    part_index += 1


def LoadJSONWithComments(filename):
  """Loads a JSON file ignoring lines with comments.

  RFC 7159 doesn't allow comments on the file JSON format. This functions loads
  a JSON file removing all the comment lines. A comment line is any line
  starting with # and optionally indented with whitespaces. Note that inline
  comments are not supported.

  Args:
    filename: The input filename.

  Returns:
    The parsed JSON object.
  """
  regex = re.compile(r'^\s*#.*')
  with open(filename) as f:
    source = ''.join(regex.sub('', line) for line in f)
  return json.loads(source)


def _LoadStackedPartitionConfig(filename):
  """Loads a partition table and its possible parent tables.

  This does very little validation.  It's just enough to walk all of the parent
  files and merges them with the current config.  Overall validation is left to
  the caller.

  Args:
    filename: Filename to load into object.

  Returns:
    Object containing disk layout configuration
  """
  if not os.path.exists(filename):
    raise ConfigNotFound('Partition config %s was not found!' % filename)
  config = LoadJSONWithComments(filename)

  # Let's first apply our new configs onto base.
  common_layout = config['layouts'].setdefault(COMMON_LAYOUT, [])
  for layout_name, layout in config['layouts'].iteritems():
    # Don't apply on yourself.
    if layout_name == COMMON_LAYOUT or layout_name == '_comment':
      continue

    # Need to copy a list of dicts so make a deep copy.
    working_layout = copy.deepcopy(common_layout)
    _ApplyLayoutOverrides(working_layout, layout)
    config['layouts'][layout_name] = working_layout

  dirname = os.path.dirname(filename)
  # Now let's inherit the values from all our parents.
  for parent in config.get('parent', '').split():
    parent_filename = os.path.join(dirname, parent)
    if not os.path.exists(parent_filename):
      # Try loading the parent file from the cgpt.py directory (global config).
      parent_filename = os.path.join(os.path.join(os.path.dirname(__file__),
                                                  parent))
    parent_config = _LoadStackedPartitionConfig(parent_filename)

    # First if the parent is missing any fields the new config has, fill them
    # in.
    for key in config.keys():
      if key == 'parent':
        continue
      elif key == 'metadata':
        # We handle this especially to allow for inner metadata fields to be
        # added / modified.
        parent_config.setdefault(key, {})
        parent_config[key].update(config[key])
      else:
        parent_config.setdefault(key, config[key])

    # The overrides work by taking the parent_config, apply the new config
    # layout info, and return the resulting config which is stored in the parent
    # config.

    # So there's an issue where an inheriting layout file may contain new
    # layouts not previously defined in the parent layout. Since we are
    # building these layout files based on the parent configs and overriding
    # new values, we first add the new layouts not previously defined in the
    # parent config using a copy of the base layout from that parent config.
    parent_layouts = set(parent_config['layouts'])
    config_layouts = set(config['layouts'])
    new_layouts = config_layouts - parent_layouts

    # Actually add the copy. Use a copy such that each is unique.
    parent_cmn_layout = parent_config['layouts'].setdefault(COMMON_LAYOUT, [])
    for layout_name in new_layouts:
      parent_config['layouts'][layout_name] = copy.deepcopy(parent_cmn_layout)

    # Iterate through each layout in the parent config and apply the new layout.
    common_layout = config['layouts'].setdefault(COMMON_LAYOUT, [])
    for layout_name, parent_layout in parent_config['layouts'].iteritems():
      if layout_name == '_comment':
        continue

      layout_override = config['layouts'].setdefault(layout_name, [])
      if layout_name != COMMON_LAYOUT:
        _ApplyLayoutOverrides(parent_layout, common_layout)

      _ApplyLayoutOverrides(parent_layout, layout_override)

    config = parent_config

  config.pop('parent', None)
  return config


def LoadPartitionConfig(filename):
  """Loads a partition tables configuration file into a Python object.

  Args:
    filename: Filename to load into object

  Returns:
    Object containing disk layout configuration
  """

  valid_keys = set(('_comment', 'metadata', 'layouts', 'parent'))
  valid_layout_keys = set((
      '_comment', 'num', 'blocks', 'block_size', 'fs_blocks', 'fs_block_size',
      'uuid', 'label', 'format', 'fs_format', 'type', 'features',
      'size', 'fs_size', 'fs_options', 'erase_block_size', 'hybrid_mbr',
      'reserved_erase_blocks', 'max_bad_erase_blocks', 'external_gpt',
      'page_size', 'size_min', 'fs_size_min'))
  valid_features = set(('expand',))

  config = _LoadStackedPartitionConfig(filename)
  try:
    metadata = config['metadata']
    for key in ('block_size', 'fs_block_size'):
      metadata[key] = ParseHumanNumber(metadata[key])

    unknown_keys = set(config.keys()) - valid_keys
    if unknown_keys:
      raise InvalidLayout('Unknown items: %r' % unknown_keys)

    if len(config['layouts']) <= 0:
      raise InvalidLayout('Missing "layouts" entries')

    if not BASE_LAYOUT in config['layouts'].keys():
      raise InvalidLayout('Missing "base" config in "layouts"')

    for layout_name, layout in config['layouts'].iteritems():
      if layout_name == '_comment':
        continue

      for part in layout:
        unknown_keys = set(part.keys()) - valid_layout_keys
        if unknown_keys:
          raise InvalidLayout('Unknown items in layout %s: %r' %
                              (layout_name, unknown_keys))

        if part.get('num') == 'metadata' and 'type' not in part:
          part['type'] = 'blank'

        if part['type'] != 'blank':
          for s in ('num', 'label'):
            if not s in part:
              raise InvalidLayout('Layout "%s" missing "%s"' % (layout_name, s))

        if 'size' in part:
          if 'blocks' in part:
            raise ConflictingOptions(
                '%s: Conflicting settings are used. '
                'Found section sets both \'blocks\' and \'size\'.' %
                part['label'])
          part['bytes'] = ParseHumanNumber(part['size'])
          if 'size_min' in part:
            size_min = ParseHumanNumber(part['size_min'])
            if part['bytes'] < size_min:
              part['bytes'] = size_min
          part['blocks'] = part['bytes'] / metadata['block_size']

          if part['bytes'] % metadata['block_size'] != 0:
            raise InvalidSize(
                'Size: "%s" (%s bytes) is not an even number of block_size: %s'
                % (part['size'], part['bytes'], metadata['block_size']))

        if 'fs_size' in part:
          part['fs_bytes'] = ParseHumanNumber(part['fs_size'])
          if 'fs_size_min' in part:
            fs_size_min = ParseHumanNumber(part['fs_size_min'])
            if part['fs_bytes'] < fs_size_min:
              part['fs_bytes'] = fs_size_min
          if part['fs_bytes'] <= 0:
            raise InvalidSize(
                'File system size "%s" must be positive' %
                part['fs_size'])
          if part['fs_bytes'] > part['bytes']:
            raise InvalidSize(
                'Filesystem may not be larger than partition: %s %s: %d > %d' %
                (layout_name, part['label'], part['fs_bytes'], part['bytes']))
          if part['fs_bytes'] % metadata['fs_block_size'] != 0:
            raise InvalidSize(
                'File system size: "%s" (%s bytes) is not an even number of '
                'fs blocks: %s' %
                (part['fs_size'], part['fs_bytes'], metadata['fs_block_size']))
          if part.get('format') == 'ubi':
            part_meta = GetMetadataPartition(layout)
            page_size = ParseHumanNumber(part_meta['page_size'])
            eb_size = ParseHumanNumber(part_meta['erase_block_size'])
            ubi_eb_size = eb_size - 2 * page_size
            if (part['fs_bytes'] % ubi_eb_size) != 0:
              # Trim fs_bytes to multiple of UBI eraseblock size.
              fs_bytes = part['fs_bytes'] - (part['fs_bytes'] % ubi_eb_size)
              raise InvalidSize(
                  'File system size: "%s" (%d bytes) is not a multiple of UBI '
                  'erase block size (%d). Please set "fs_size" to "%s" in the '
                  '"common" layout instead.' %
                  (part['fs_size'], part['fs_bytes'], ubi_eb_size,
                   ProduceHumanNumber(fs_bytes)))

        if 'blocks' in part:
          part['blocks'] = ParseHumanNumber(part['blocks'])
          part['bytes'] = part['blocks'] * metadata['block_size']

        if 'fs_blocks' in part:
          max_fs_blocks = part['bytes'] / metadata['fs_block_size']
          part['fs_blocks'] = ParseRelativeNumber(max_fs_blocks,
                                                  part['fs_blocks'])
          part['fs_bytes'] = part['fs_blocks'] * metadata['fs_block_size']

          if part['fs_bytes'] > part['bytes']:
            raise InvalidLayout(
                'Filesystem may not be larger than partition: %s %s: %d > %d' %
                (layout_name, part['label'], part['fs_bytes'], part['bytes']))
        if 'erase_block_size' in part:
          part['erase_block_size'] = ParseHumanNumber(part['erase_block_size'])
        if 'page_size' in part:
          part['page_size'] = ParseHumanNumber(part['page_size'])

        part.setdefault('features', [])
        unknown_features = set(part['features']) - valid_features
        if unknown_features:
          raise InvalidLayout('%s: Unknown features: %s' %
                              (part['label'], unknown_features))
  except KeyError as e:
    raise InvalidLayout('Layout is missing required entries: %s' % e)

  return config


def _GetPrimaryEntryArrayLBA(config):
  """Return the start LBA of the primary partition entry array.

  Normally this comes after the primary GPT header but can be adjusted by
  setting the "primary_entry_array_lba" key under "metadata" in the config.

  Args:
    config: The config dictionary.

  Returns:
    The position of the primary partition entry array.
  """

  pmbr_and_header_size = SIZE_OF_PMBR + SIZE_OF_GPT_HEADER
  entry_array = config['metadata'].get('primary_entry_array_lba',
                                       pmbr_and_header_size)
  if entry_array < pmbr_and_header_size:
    raise InvalidLayout('Primary entry array (%d) must be at least %d.' %
                        entry_array, pmbr_and_header_size)
  return entry_array


def _HasBadEraseBlocks(partitions):
  return 'max_bad_erase_blocks' in GetMetadataPartition(partitions)


def _HasExternalGpt(partitions):
  return GetMetadataPartition(partitions).get('external_gpt', False)


def _GetStartSector(config, partitions):
  """Return the first usable location (LBA) for partitions.

  This value is the first LBA after the PMBR, the primary GPT header, and
  partition entry array.

  We round it up to 64 to maintain the same layout as before in the normal (no
  padding between the primary GPT header and its partition entry array) case.

  Args:
    config: The config dictionary.
    partitions: List of partitions to process

  Returns:
    A suitable LBA for partitions, at least 64.
  """

  if _HasExternalGpt(partitions):
    # If the GPT is external, then the offset of the partitions' actual data
    # will be 0, and we don't need to make space at the beginning for the GPT.
    return 0
  else:
    entry_array = _GetPrimaryEntryArrayLBA(config)
    start_sector = max(entry_array + SIZE_OF_PARTITION_ENTRY_ARRAY, 64)
    return start_sector


def GetTableTotals(config, partitions):
  """Calculates total sizes/counts for a partition table.

  Args:
    config: Partition configuration file object
    partitions: List of partitions to process

  Returns:
    Dict containing totals data
  """

  start_sector = _GetStartSector(config, partitions)
  ret = {
      'expand_count': 0,
      'expand_min': 0,
      'block_count': start_sector * config['metadata']['block_size']
  }

  # Total up the size of all non-expanding partitions to get the minimum
  # required disk size.
  for partition in partitions:
    if partition.get('num') == 'metadata':
      continue
    if 'expand' in partition['features']:
      ret['expand_count'] += 1
      ret['expand_min'] += partition['blocks']
    else:
      ret['block_count'] += partition['blocks']

  # At present, only one expanding partition is permitted.
  # Whilst it'd be possible to have two, we don't need this yet
  # and it complicates things, so it's been left out for now.
  if ret['expand_count'] > 1:
    raise InvalidLayout('1 expand partition allowed, %d requested'
                        % ret['expand_count'])

  ret['min_disk_size'] = ret['block_count'] + ret['expand_min']

  return ret


def GetPartitionTable(options, config, image_type):
  """Generates requested image_type layout from a layout configuration.

  This loads the base table and then overlays the requested layout over
  the base layout.

  Args:
    options: Flags passed to the script
    config: Partition configuration file object
    image_type: Type of image eg base/test/dev/factory_install

  Returns:
    Object representing a selected partition table
  """

  # We make a deep copy so that changes to the dictionaries in this list do not
  # persist across calls.
  partitions = copy.deepcopy(config['layouts'][image_type])
  metadata = config['metadata']

  # Convert fs_options to a string.
  for partition in partitions:
    fs_options = partition.get('fs_options', '')
    if isinstance(fs_options, dict):
      fs_format = partition.get('fs_format')
      fs_options = fs_options.get(fs_format, '')
    elif not isinstance(fs_options, basestring):
      raise InvalidLayout('Partition number %s: fs_format must be a string or '
                          'dict, not %s' % (partition.get('num'),
                                            type(fs_options)))
    if '"' in fs_options or "'" in fs_options:
      raise InvalidLayout('Partition number %s: fs_format cannot have quotes' %
                          partition.get('num'))
    partition['fs_options'] = fs_options

  for adjustment_str in options.adjust_part.split():
    adjustment = adjustment_str.split(':')
    if len(adjustment) < 2:
      raise InvalidAdjustment('Adjustment "%s" is incomplete' % adjustment_str)

    label = adjustment[0]
    operator = adjustment[1][0]
    operand = adjustment[1][1:]
    ApplyPartitionAdjustment(partitions, metadata, label, operator, operand)

  return partitions


def ApplyPartitionAdjustment(partitions, metadata, label, operator, operand):
  """Applies an adjustment to a partition specified by label

  Args:
    partitions: Partition table to modify
    metadata: Partition table metadata
    label: The label of the partition to adjust
    operator: Type of adjustment (+/-/=)
    operand: How much to adjust by
  """

  partition = GetPartitionByLabel(partitions, label)

  operand_bytes = ParseHumanNumber(operand)
  if operand_bytes % metadata['block_size'] == 0:
    operand_blocks = operand_bytes / metadata['block_size']
  else:
    raise InvalidAdjustment('Adjustment size %s not divisible by block size %s'
                            % (operand_bytes, metadata['block_size']))

  if operator == '+':
    partition['blocks'] += operand_blocks
    partition['bytes'] += operand_bytes
  elif operator == '-':
    partition['blocks'] -= operand_blocks
    partition['bytes'] -= operand_bytes
  elif operator == '=':
    partition['blocks'] = operand_blocks
    partition['bytes'] = operand_bytes
  else:
    raise ValueError('unknown operator %s' % operator)

  if partition['type'] == 'rootfs':
    # If we're adjusting a rootFS partition, we assume the full partition size
    # specified is being used for the filesytem, minus the space reserved for
    # the hashpad.
    partition['fs_bytes'] = partition['bytes']
    partition['fs_blocks'] = partition['fs_bytes'] / metadata['fs_block_size']
    partition['blocks'] = int(partition['blocks'] * 1.15)
    partition['bytes'] = partition['blocks'] * metadata['block_size']


def GetPartitionTableFromConfig(options, layout_filename, image_type):
  """Loads a partition table and returns a given partition table type

  Args:
    options: Flags passed to the script
    layout_filename: The filename to load tables from
    image_type: The type of partition table to return
  """

  config = LoadPartitionConfig(layout_filename)
  partitions = GetPartitionTable(options, config, image_type)

  return partitions


def GetScriptShell():
  """Loads and returns the skeleton script for our output script.

  Returns:
    A string containing the skeleton script
  """

  script_shell_path = os.path.join(os.path.dirname(__file__), 'cgpt_shell.sh')
  with open(script_shell_path, 'r') as f:
    script_shell = ''.join(f.readlines())

  # Before we return, insert the path to this tool so somebody reading the
  # script later can tell where it was generated.
  script_shell = script_shell.replace('@SCRIPT_GENERATOR@', script_shell_path)

  return script_shell


def GetFullPartitionSize(partition, metadata):
  """Get the size of the partition including metadata/reserved space in bytes.

  The partition only has to be bigger for raw NAND devices. Formula:
  - Add UBI per-block metadata (2 pages) if partition is UBI
  - Round up to erase block size
  - Add UBI per-partition metadata (4 blocks) if partition is UBI
  - Add reserved erase blocks
  """

  erase_block_size = metadata.get('erase_block_size', 0)
  size = partition['bytes']

  if erase_block_size == 0:
    return size

  # See "Flash space overhead" in
  # http://www.linux-mtd.infradead.org/doc/ubi.html
  # for overhead calculations.
  is_ubi = partition.get('format') == 'ubi'
  reserved_erase_blocks = partition.get('reserved_erase_blocks', 0)
  page_size = metadata.get('page_size', 0)

  if is_ubi:
    ubi_block_size = erase_block_size - 2 * page_size
    erase_blocks = (size + ubi_block_size - 1) // ubi_block_size
    size += erase_blocks * 2 * page_size

  erase_blocks = (size + erase_block_size - 1) // erase_block_size
  size = erase_blocks * erase_block_size

  if is_ubi:
    size += erase_block_size * 4

  size += reserved_erase_blocks * erase_block_size
  return size


def WriteLayoutFunction(options, sfile, func, image_type, config):
  """Writes a shell script function to write out a given partition table.

  Args:
    options: Flags passed to the script
    sfile: File handle we're writing to
    func: function of the layout:
       for removable storage device: 'partition',
       for the fixed storage device: 'base'
    image_type: Type of image eg base/test/dev/factory_install
    config: Partition configuration file object
  """

  partitions = GetPartitionTable(options, config, image_type)
  metadata = GetMetadataPartition(partitions)
  partition_totals = GetTableTotals(config, partitions)

  lines = [
      'write_%s_table() {' % func,
  ]

  if _HasExternalGpt(partitions):
    # Read GPT from device to get size, then wipe it out and operate
    # on GPT in tmpfs. We don't rely on cgpt's ability to deal
    # directly with the GPT on SPI NOR flash because rewriting the
    # table so many times would take a long time (>30min).
    # Also, wiping out the previous GPT with create_image won't work
    # for NAND and there's no equivalent via cgpt.
    lines += [
        'gptfile=$(mktemp)',
        'flashrom -r -iRW_GPT:${gptfile}',
        'gptsize=$(stat ${gptfile} --format %s)',
        'dd if=/dev/zero of=${gptfile} bs=${gptsize} count=1',
        'target="-D %d ${gptfile}"' % metadata['bytes'],
    ]
  else:
    lines += [
        'local target="$1"',
        'create_image "${target}" %d %s' % (
            partition_totals['min_disk_size'],
            config['metadata']['block_size']),
    ]

  # ${target} is referenced unquoted because it may expand into multiple
  # arguments in the case of NAND
  lines += [
      'local curr=%d' % _GetStartSector(config, partitions),
      '# Create the GPT headers and tables. Pad the primary ones.',
      './cgpt create -p %d ${target}' % (_GetPrimaryEntryArrayLBA(config) -
                                         (SIZE_OF_PMBR + SIZE_OF_GPT_HEADER)),
  ]

  metadata = GetMetadataPartition(partitions)
  # Pass 1: Set up the expanding partition size.
  for partition in partitions:
    if partition.get('num') == 'metadata':
      continue
    partition['var'] = (GetFullPartitionSize(partition, metadata) /
                        config['metadata']['block_size'])

    if (partition.get('type') != 'blank' and partition['num'] == 1 and
        'expand' in partition['features']):
      lines += [
          'local stateful_size=%s' % partition['blocks'],
          'if [ -b "${target}" ]; then',
          '  stateful_size=$(( $(numsectors "${target}") - %d))' % (
              partition_totals['block_count']),
          'fi',
          ': $(( stateful_size -= (stateful_size %% %d) ))' % (
              config['metadata']['fs_block_size']),
      ]
      partition['var'] = '${stateful_size}'

  # Pass 2: Write out all the cgpt add commands.
  for partition in partitions:
    if partition.get('num') == 'metadata':
      continue
    if partition['type'] != 'blank':
      lines += [
          './cgpt add -i %d -b ${curr} -s %s -t %s -l "%s" ${target}' % (
              partition['num'], str(partition['var']), partition['type'],
              partition['label']),
      ]

    # Increment the curr counter ready for the next partition.
    if partition['var'] != 0 and partition.get('num') != 'metadata':
      lines += [
          ': $(( curr += %s ))' % partition['var'],
      ]

  # Set default priorities and retry counter on kernel partitions.
  tries = 15
  prio = 15
  # The order of partition numbers in this loop matters.
  # Make sure partition #2 is the first one, since it will be marked as
  # default bootable partition.
  for partition in GetPartitionsByType(partitions, 'kernel'):
    lines += [
        './cgpt add -i %s -S 0 -T %i -P %i ${target}' %
        (partition['num'], tries, prio)
    ]
    prio = 0
    # When not writing 'base' function, make sure the other partitions are
    # marked as non-bootable (retry count == 0), since the USB layout
    # doesn't have any valid data in slots B & C. But with base function,
    # called by chromeos-install script, the KERNEL A partition is replicated
    # into both slots A & B, so we should leave both bootable for error
    # recovery in this case.
    if func != 'base':
      tries = 0

  efi_partitions = GetPartitionsByType(partitions, 'efi')
  if efi_partitions:
    lines += [
        './cgpt boot -p -b $2 -i %d ${target}' % efi_partitions[0]['num'],
        './cgpt add -i %s -B 1 ${target}' % efi_partitions[0]['num'],
    ]
  else:
    # Provide a PMBR all the time for boot loaders (like u-boot)
    # that expect one to always be there.
    lines += [
        './cgpt boot -p -b $2 ${target}',
    ]

  if metadata.get('hybrid_mbr'):
    lines += ['install_hybrid_mbr ${target}']
  lines += ['./cgpt show ${target}']

  if _HasExternalGpt(partitions):
    lines += ['flashrom -w -iRW_GPT:${gptfile} --fast-verify']

  sfile.write('%s\n}\n' % '\n  '.join(lines))


def WritePartitionSizesFunction(options, sfile, func, image_type, config):
  """Writes out the partition size variable that can be extracted by a caller.

  Args:
    options: Flags passed to the script
    sfile: File handle we're writing to
    func: function of the layout:
       for removable storage device: 'partition',
       for the fixed storage device: 'base'
    image_type: Type of image eg base/test/dev/factory_install
    config: Partition configuration file object
  """
  func_name = 'load_%s_vars' % func
  lines = [
      '%s() {' % func_name,
      'DEFAULT_ROOTDEV="%s"' % config['metadata'].get('rootdev_%s' % func, ''),
  ]

  partitions = GetPartitionTable(options, config, image_type)
  for partition in partitions:
    if partition.get('num') == 'metadata':
      continue
    for key in ('label', 'num'):
      if key in partition:
        shell_label = str(partition[key]).replace('-', '_').upper()
        part_bytes = partition['bytes']
        reserved_ebs = partition.get('reserved_erase_blocks', 0)
        fs_bytes = partition.get('fs_bytes', part_bytes)
        part_format = partition.get('format', '')
        fs_format = partition.get('fs_format', '')
        fs_options = partition.get('fs_options', '')
        partition_num = partition.get('num', '')
        lines += [
            'PARTITION_SIZE_%s=%s' % (shell_label, part_bytes),
            '  RESERVED_EBS_%s=%s' % (shell_label, reserved_ebs),
            '     DATA_SIZE_%s=%s' % (shell_label, fs_bytes),
            '        FORMAT_%s=%s' % (shell_label, part_format),
            '     FS_FORMAT_%s=%s' % (shell_label, fs_format),
            '    FS_OPTIONS_%s="%s"' % (shell_label, fs_options),
            ' PARTITION_NUM_%s="%s"' % (shell_label, partition_num),
        ]

  sfile.write('%s\n}\n' % '\n  '.join(lines))


def GetPartitionByNumber(partitions, num):
  """Given a partition table and number returns the partition object.

  Args:
    partitions: List of partitions to search in
    num: Number of partition to find

  Returns:
    An object for the selected partition
  """
  for partition in partitions:
    if partition.get('num') == int(num):
      return partition

  raise PartitionNotFound('Partition %s not found' % num)


def GetPartitionsByType(partitions, typename):
  """Given a partition table and type returns the partitions of the type.

  Partitions are sorted in num order.

  Args:
    partitions: List of partitions to search in
    typename: The type of partitions to select

  Returns:
    A list of partitions of the type
  """
  out = []
  for partition in partitions:
    if partition.get('type') == typename:
      out.append(partition)
  return sorted(out, key=lambda partition: partition.get('num'))


def GetMetadataPartition(partitions):
  """Given a partition table returns the metadata partition object.

  Args:
    partitions: List of partitions to search in

  Returns:
    An object for the metadata partition
  """
  for partition in partitions:
    if partition.get('num') == 'metadata':
      return partition

  return {}


def GetPartitionByLabel(partitions, label):
  """Given a partition table and label returns the partition object.

  Args:
    partitions: List of partitions to search in
    label: Label of partition to find

  Returns:
    An object for the selected partition
  """
  for partition in partitions:
    if 'label' not in partition:
      continue
    if partition['label'] == label:
      return partition

  raise PartitionNotFound('Partition "%s" not found' % label)


def WritePartitionScript(options, image_type, layout_filename, sfilename):
  """Writes a shell script with functions for the base and requested layouts.

  Args:
    options: Flags passed to the script
    image_type: Type of image eg base/test/dev/factory_install
    layout_filename: Path to partition configuration file
    sfilename: Filename to write the finished script to
  """
  config = LoadPartitionConfig(layout_filename)

  with open(sfilename, 'w') as f:
    script_shell = GetScriptShell()
    f.write(script_shell)

    for func, layout in (('base', BASE_LAYOUT), ('partition', image_type)):
      WriteLayoutFunction(options, f, func, layout, config)
      WritePartitionSizesFunction(options, f, func, layout, config)

    # TODO: Backwards compat.  Should be killed off once we update
    #       cros_generate_update_payload to use the new code.
    partitions = GetPartitionTable(options, config, BASE_LAYOUT)
    partition = GetPartitionByLabel(partitions, 'ROOT-A')
    f.write('ROOTFS_PARTITION_SIZE=%s\n' % (partition['bytes'],))


def GetBlockSize(_options, layout_filename):
  """Returns the partition table block size.

  Args:
    options: Flags passed to the script
    layout_filename: Path to partition configuration file

  Returns:
    Block size of all partitions in the layout
  """

  config = LoadPartitionConfig(layout_filename)
  return config['metadata']['block_size']


def GetFilesystemBlockSize(_options, layout_filename):
  """Returns the filesystem block size.

  This is used for all partitions in the table that have filesystems.

  Args:
    options: Flags passed to the script
    layout_filename: Path to partition configuration file

  Returns:
    Block size of all filesystems in the layout
  """

  config = LoadPartitionConfig(layout_filename)
  return config['metadata']['fs_block_size']


def GetImageTypes(_options, layout_filename):
  """Returns a list of all the image types in the layout.

  Args:
    options: Flags passed to the script
    layout_filename: Path to partition configuration file

  Returns:
    List of all image types
  """

  config = LoadPartitionConfig(layout_filename)
  return ' '.join(config['layouts'].keys())


def GetType(options, image_type, layout_filename, num):
  """Returns the type of a given partition for a given layout.

  Args:
    options: Flags passed to the script
    image_type: Type of image eg base/test/dev/factory_install
    layout_filename: Path to partition configuration file
    num: Number of the partition you want to read from

  Returns:
    Type of the specified partition.
  """
  partitions = GetPartitionTableFromConfig(options, layout_filename, image_type)
  partition = GetPartitionByNumber(partitions, num)
  return partition.get('type')


def GetPartitions(options, image_type, layout_filename):
  """Returns the partition numbers for the image_type.

  Args:
    options: Flags passed to the script
    image_type: Type of image eg base/test/dev/factory_install
    layout_filename: Path to partition configuration file

  Returns:
    A space delimited string of partition numbers.
  """
  partitions = GetPartitionTableFromConfig(options, layout_filename, image_type)
  return ' '.join(str(p['num']) for p in partitions
                  if 'num' in p and p['num'] != 'metadata')


def GetUUID(options, image_type, layout_filename, num):
  """Returns the filesystem UUID of a given partition for a given layout type.

  Args:
    options: Flags passed to the script
    image_type: Type of image eg base/test/dev/factory_install
    layout_filename: Path to partition configuration file
    num: Number of the partition you want to read from

  Returns:
    UUID of specified partition. Defaults to random if not set.
  """
  partitions = GetPartitionTableFromConfig(options, layout_filename, image_type)
  partition = GetPartitionByNumber(partitions, num)
  return partition.get('uuid', 'random')


def GetPartitionSize(options, image_type, layout_filename, num):
  """Returns the partition size of a given partition for a given layout type.

  Args:
    options: Flags passed to the script
    image_type: Type of image eg base/test/dev/factory_install
    layout_filename: Path to partition configuration file
    num: Number of the partition you want to read from

  Returns:
    Size of selected partition in bytes
  """

  partitions = GetPartitionTableFromConfig(options, layout_filename, image_type)
  partition = GetPartitionByNumber(partitions, num)

  return partition['bytes']


def GetFilesystemFormat(options, image_type, layout_filename, num):
  """Returns the filesystem format of a given partition for a given layout type.

  Args:
    options: Flags passed to the script
    image_type: Type of image eg base/test/dev/factory_install
    layout_filename: Path to partition configuration file
    num: Number of the partition you want to read from

  Returns:
    Format of the selected partition's filesystem
  """

  partitions = GetPartitionTableFromConfig(options, layout_filename, image_type)
  partition = GetPartitionByNumber(partitions, num)

  return partition.get('fs_format')


def GetFormat(options, image_type, layout_filename, num):
  """Returns the format of a given partition for a given layout type.

  Args:
    options: Flags passed to the script
    image_type: Type of image eg base/test/dev/factory_install
    layout_filename: Path to partition configuration file
    num: Number of the partition you want to read from

  Returns:
    Format of the selected partition's filesystem
  """

  partitions = GetPartitionTableFromConfig(options, layout_filename, image_type)
  partition = GetPartitionByNumber(partitions, num)

  return partition.get('format')


def GetFilesystemOptions(options, image_type, layout_filename, num):
  """Returns the filesystem options of a given partition and layout type.

  Args:
    options: Flags passed to the script
    image_type: Type of image eg base/test/dev/factory_install
    layout_filename: Path to partition configuration file
    num: Number of the partition you want to read from

  Returns:
    The selected partition's filesystem options
  """

  partitions = GetPartitionTableFromConfig(options, layout_filename, image_type)
  partition = GetPartitionByNumber(partitions, num)

  return partition.get('fs_options')


def GetFilesystemSize(options, image_type, layout_filename, num):
  """Returns the filesystem size of a given partition for a given layout type.

  If no filesystem size is specified, returns the partition size.

  Args:
    options: Flags passed to the script
    image_type: Type of image eg base/test/dev/factory_install
    layout_filename: Path to partition configuration file
    num: Number of the partition you want to read from

  Returns:
    Size of selected partition filesystem in bytes
  """

  partitions = GetPartitionTableFromConfig(options, layout_filename, image_type)
  partition = GetPartitionByNumber(partitions, num)

  if 'fs_bytes' in partition:
    return partition['fs_bytes']
  else:
    return partition['bytes']


def GetLabel(options, image_type, layout_filename, num):
  """Returns the label for a given partition.

  Args:
    options: Flags passed to the script
    image_type: Type of image eg base/test/dev/factory_install
    layout_filename: Path to partition configuration file
    num: Number of the partition you want to read from

  Returns:
    Label of selected partition, or 'UNTITLED' if none specified
  """

  partitions = GetPartitionTableFromConfig(options, layout_filename, image_type)
  partition = GetPartitionByNumber(partitions, num)

  if 'label' in partition:
    return partition['label']
  else:
    return 'UNTITLED'


def GetNumber(options, image_type, layout_filename, label):
  """Returns the partition number of a given label.

  Args:
    options: Flags passed to the script
    image_type: Type of image eg base/test/dev/factory_install
    layout_filename: Path to partition configuration file
    label: Number of the partition you want to read from

  Returns:
    The number of the partition corresponding to the label.
  """

  partitions = GetPartitionTableFromConfig(options, layout_filename, image_type)
  partition = GetPartitionByLabel(partitions, label)
  return partition['num']


def GetReservedEraseBlocks(options, image_type, layout_filename, num):
  """Returns the number of erase blocks reserved in the partition.

  Args:
    options: Flags passed to the script
    image_type: Type of image eg base/test/dev/factory_install
    layout_filename: Path to partition configuration file
    num: Number of the partition you want to read from

  Returns:
    Number of reserved erase blocks
  """
  partitions = GetPartitionTableFromConfig(options, layout_filename, image_type)
  partition = GetPartitionByNumber(partitions, num)
  if 'reserved_erase_blocks' in partition:
    return partition['reserved_erase_blocks']
  else:
    return 0


def DoDebugOutput(options, image_type, layout_filename):
  """Prints out a human readable disk layout in on-disk order.

  Args:
    options: Flags passed to the script
    image_type: Type of image eg base/test/dev/factory_install
    layout_filename: Path to partition configuration file
  """
  config = LoadPartitionConfig(layout_filename)
  partitions = GetPartitionTable(options, config, image_type)

  label_len = max([len(x['label']) for x in partitions if 'label' in x])
  type_len = max([len(x['type']) for x in partitions if 'type' in x])

  msg = 'num:%4s label:%-*s type:%-*s size:%-10s fs_size:%-10s features:%s'

  # Print out non-layout options first.
  print('Config Data')
  metadata_msg = 'field:%-14s value:%s'
  for key in config.keys():
    if key not in ('layouts', '_comment'):
      print(metadata_msg % (key, config[key]))

  print('\n%s Layout Data' % image_type.upper())
  for partition in partitions:
    if partition.get('num') == 'metadata':
      continue

    size = ProduceHumanNumber(partition['bytes'])
    if 'fs_bytes' in partition.iterkeys():
      fs_size = ProduceHumanNumber(partition['fs_bytes'])
    else:
      fs_size = 'auto'

    print(msg % (
        partition.get('num', 'auto'),
        label_len,
        partition.get('label', ''),
        type_len,
        partition.get('type', ''),
        size,
        fs_size,
        partition.get('features', []),
    ))


def CheckRootfsPartitionsMatch(partitions):
  """Checks that rootfs partitions are substitutable with each other.

  This function asserts that either all rootfs partitions are in the same format
  or none have a format, and it asserts that have the same number of reserved
  erase blocks.
  """
  partition_format = None
  reserved_erase_blocks = -1
  for partition in partitions:
    if partition.get('type') == 'rootfs':
      new_format = partition.get('format', '')
      new_reserved_erase_blocks = partition.get('reserved_erase_blocks', 0)

      if partition_format is None:
        partition_format = new_format
        reserved_erase_blocks = new_reserved_erase_blocks

      if new_format != partition_format:
        raise MismatchedRootfsFormat(
            'mismatched rootfs formats: "%s" and "%s"' %
            (partition_format, new_format))

      if reserved_erase_blocks != new_reserved_erase_blocks:
        raise MismatchedRootfsBlocks(
            'mismatched rootfs reserved erase block counts: %s and %s' %
            (reserved_erase_blocks, new_reserved_erase_blocks))


def Combinations(n, k):
  """Calculate the binomial coefficient, i.e., "n choose k"

  This calculates the number of ways that k items can be chosen from
  a set of size n. For example, if there are n blocks and k of them
  are bad, then this returns the number of ways that the bad blocks
  can be distributed over the device.
  See http://en.wikipedia.org/wiki/Binomial_coefficient

  For convenience to the caller, this function allows impossible cases
  as input and returns 0 for them.
  """
  if k < 0 or n < k:
    return 0
  return math.factorial(n) / (math.factorial(k) * math.factorial(n - k))


def CheckReservedEraseBlocks(partitions):
  """Checks that the reserved_erase_blocks in each partition is good.

  This function checks that a reasonable value was given for the reserved
  erase block count. In particular, it checks that there's a less than
  1 in 100k probability that, if the manufacturer's maximum bad erase
  block count is met, and assuming bad blocks are uniformly randomly
  distributed, then more bad blocks will fall in this partition than are
  reserved. Smaller partitions need a larger reserve percentage.

  We take the number of reserved blocks as a parameter in disk_layout.json
  rather than just calculating the value so that it can be tweaked
  explicitly along with others in squeezing the image onto flash. But
  we check it so that users have an easy method for determining what's
  acceptable--just try out a new value and do ./build_image.
  """
  for partition in partitions:
    if ('reserved_erase_blocks' in partition or
        partition.get('format') in ('ubi', 'nand')):
      if partition.get('bytes', 0) == 0:
        continue
      metadata = GetMetadataPartition(partitions)
      if (not _HasBadEraseBlocks(partitions)
          or 'reserved_erase_blocks' not in partition
          or 'bytes' not in metadata
          or 'erase_block_size' not in metadata
          or 'page_size' not in metadata):
        raise MissingEraseBlockField(
            'unable to check if partition %s will have too many bad blocks due '
            'to missing metadata field' % partition['label'])

      reserved = partition['reserved_erase_blocks']
      erase_block_size = metadata['erase_block_size']
      device_erase_blocks = metadata['bytes'] / erase_block_size
      device_bad_blocks = metadata['max_bad_erase_blocks']
      distributions = Combinations(device_erase_blocks, device_bad_blocks)
      partition_erase_blocks = partition['bytes'] / erase_block_size
      # The idea is to calculate the number of ways that there could be reserved
      # or more bad blocks inside the partition, assuming that there are
      # device_bad_blocks in the device in total (the worst case). To get the
      # probability, we divide this count by the total number of ways that the
      # bad blocks can be distributed on the whole device. To find the first
      # number, we sum over increasing values for the count of bad blocks within
      # the partition the number of ways that those bad blocks can be inside the
      # partition, multiplied by the number of ways that the remaining blocks
      # can be distributed outside of the partition.
      ways_for_failure = sum(
          Combinations(partition_erase_blocks, partition_bad_blocks) *
          Combinations(device_erase_blocks - partition_erase_blocks,
                       device_bad_blocks - partition_bad_blocks)
          for partition_bad_blocks
          in range(reserved + 1, device_bad_blocks + 1))
      probability = (1.0 * ways_for_failure) / distributions
      if probability > 0.00001:
        raise ExcessFailureProbability('excessive probability %f of too many '
                                       'bad blocks in partition %s'
                                       % (probability, partition['label']))


def CheckSimpleNandProperties(partitions):
  """Checks that NAND partitions are erase-block-aligned and not expand"""
  if not _HasBadEraseBlocks(partitions):
    return
  metadata = GetMetadataPartition(partitions)
  for partition in partitions:
    erase_block_size = metadata['erase_block_size']
    if partition['bytes'] % erase_block_size != 0:
      raise UnalignedPartition(
          'partition size %s does not divide erase block size %s' %
          (partition['bytes'], erase_block_size))
    if 'expand' in partition['features']:
      raise ExpandNandImpossible(
          'expand partitions may not be used with raw NAND')


def CheckTotalSize(partitions):
  """Checks that the sum size of all partitions fits within the device"""
  metadata = GetMetadataPartition(partitions)
  if 'bytes' not in metadata:
    return
  capacity = metadata['bytes']
  total = sum(GetFullPartitionSize(partition, metadata)
              for partition in partitions if partition.get('num') != 'metadata')
  if total > capacity:
    raise ExcessPartitionSize('capacity = %d, total=%d' % (capacity, total))


def Validate(options, image_type, layout_filename):
  """Validates a layout file, used before reading sizes to check for errors.

  Args:
    options: Flags passed to the script
    image_type: Type of image eg base/test/dev/factory_install
    layout_filename: Path to partition configuration file
  """
  partitions = GetPartitionTableFromConfig(options, layout_filename, image_type)
  CheckRootfsPartitionsMatch(partitions)
  CheckTotalSize(partitions)
  CheckSimpleNandProperties(partitions)
  CheckReservedEraseBlocks(partitions)


def main(argv):
  action_map = {
      'write': {
          'usage': ['<image_type>', '<disk_layout>', '<script_file>'],
          'func': WritePartitionScript,
      },
      'readblocksize': {
          'usage': ['<disk_layout>'],
          'func': GetBlockSize,
      },
      'readfsblocksize': {
          'usage': ['<disk_layout>'],
          'func': GetFilesystemBlockSize,
      },
      'readpartsize': {
          'usage': ['<image_type>', '<disk_layout>', '<partition_num>'],
          'func': GetPartitionSize,
      },
      'readformat': {
          'usage': ['<image_type>', '<disk_layout>', '<partition_num>'],
          'func': GetFormat,
      },
      'readfsformat': {
          'usage': ['<image_type>', '<disk_layout>', '<partition_num>'],
          'func': GetFilesystemFormat,
      },
      'readfssize': {
          'usage': ['<image_type>', '<disk_layout>', '<partition_num>'],
          'func': GetFilesystemSize,
      },
      'readimagetypes': {
          'usage': ['<disk_layout>'],
          'func': GetImageTypes,
      },
      'readfsoptions': {
          'usage': ['<image_type>', '<disk_layout>', '<partition_num>'],
          'func': GetFilesystemOptions,
      },
      'readlabel': {
          'usage': ['<image_type>', '<disk_layout>', '<partition_num>'],
          'func': GetLabel,
      },
      'readnumber': {
          'usage': ['<image_type>', '<disk_layout>', '<partition_label>'],
          'func': GetNumber,
      },
      'readreservederaseblocks': {
          'usage': ['<image_type>', '<disk_layout>', '<partition_num>'],
          'func': GetReservedEraseBlocks,
      },
      'readtype': {
          'usage': ['<image_type>', '<disk_layout>', '<partition_num>'],
          'func': GetType,
      },
      'readpartitionnums': {
          'usage': ['<image_type>', '<disk_layout>'],
          'func': GetPartitions,
      },
      'readuuid': {
          'usage': ['<image_type>', '<disk_layout>', '<partition_num>'],
          'func': GetUUID,
      },
      'debug': {
          'usage': ['<image_type>', '<disk_layout>'],
          'func': DoDebugOutput,
      },
      'validate': {
          'usage': ['<image_type>', '<disk_layout>'],
          'func': Validate,
      },
  }

  usage = """%(prog)s <action> [options]

For information on the JSON format, see:
  http://dev.chromium.org/chromium-os/developer-guide/disk-layout-format

The --adjust_part flag takes arguments like:
  <label>:<op><size>
Where:
  <label> is a label name as found in the disk layout file
  <op> is one of the three: + - =
  <size> is a number followed by an optional size qualifier:
         B, KiB, MiB, GiB, TiB: bytes, kibi-, mebi-, gibi-, tebi- (base 1024)
         B,   K,   M,   G,   T: short hand for above
         B,  KB,  MB,  GB,  TB: bytes, kilo-, mega-, giga-, tera- (base 1000)

This will set the ROOT-A partition size to 1 gibibytes (1024 * 1024 * 1024 * 1):
  --adjust_part ROOT-A:=1GiB
This will grow the ROOT-A partition size by 500 mebibytes (1024 * 1024 * 500):
  --adjust_part ROOT-A:+500MiB
This will shrink the ROOT-A partition size by 10 mebibytes (1024 * 1024 * 10):
  --adjust_part ROOT-A:-20MiB

Actions:
"""

  action_docs = []
  for action_name, action in sorted(action_map.iteritems()):
    doc = action['func'].__doc__.split('\n', 1)[0]
    action_docs.append('  %s %s\n      %s' % (action_name,
                                              ' '.join(action['usage']), doc))
  usage += '\n\n'.join(action_docs)

  parser = argparse.ArgumentParser(usage=usage)
  parser.add_argument('--adjust_part', metavar='SPEC', default='',
                      help='adjust partition sizes')
  parser.add_argument('command', choices=sorted(action_map.keys()))
  parser.add_argument('args', nargs='*')
  opts = parser.parse_args(argv)

  action = action_map[opts.command]
  if len(action['usage']) == len(opts.args):
    ret = action['func'](opts, *opts.args)
    if ret is not None:
      print(ret)
  else:
    sys.exit('Usage: %s %s %s' % (sys.argv[0], opts.command,
                                  ' '.join(action['usage'])))


if __name__ == '__main__':
  sys.exit(main(sys.argv[1:]))
