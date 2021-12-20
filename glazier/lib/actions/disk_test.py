# Copyright 2021 Google Inc. All Rights Reserved.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#    http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
"""Tests for glazier.lib.actions.disk."""

from unittest import mock

from absl.testing import absltest
from glazier.lib.actions import disk


class DiskTest(absltest.TestCase):

  @mock.patch.object(disk.disk, 'set_disk_space', autospec=True)
  @mock.patch('glazier.lib.buildinfo.BuildInfo', autospec=True)
  def test_write_disk_size(self, bi, disk_space):
    disk.WriteDiskSize([], bi).Run()
    self.assertTrue(disk_space.called)

  @mock.patch.object(disk.disk, 'set_disk_space', autospec=True)
  @mock.patch('glazier.lib.buildinfo.BuildInfo', autospec=True)
  def test_write_disk_size_error(self, bi, disk_space):
    disk_space.side_effect = disk.ActionError
    with self.assertRaises(disk.ActionError):
      disk.WriteDiskSize([], bi).Run()


if __name__ == '__main__':
  absltest.main()