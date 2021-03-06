# Copyright 2020 Cortex Labs, Inc.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

import os
import shutil
import json
import collections
import zipfile
import pathlib
import inspect
from inspect import Parameter
from copy import deepcopy
from typing import List, Any


def has_method(object, method: str):
    return callable(getattr(object, method, None))


def extract_zip(zip_path, dest_dir=None, delete_zip_file=False):
    if dest_dir is None:
        dest_dir = os.path.dirname(zip_path)

    zip_ref = zipfile.ZipFile(zip_path, "r")
    zip_ref.extractall(dest_dir)
    zip_ref.close()

    if delete_zip_file:
        rm_file(zip_path)


def mkdir_p(dir_path):
    pathlib.Path(dir_path).mkdir(parents=True, exist_ok=True)


def rm_dir(dir_path):
    if os.path.isdir(dir_path):
        shutil.rmtree(dir_path)
        return True
    return False


def rm_file(path):
    if os.path.isfile(path):
        os.remove(path)
        return True
    return False


def trim_prefix(string, prefix):
    if string.startswith(prefix):
        return string[len(prefix) :]
    return string


def ensure_prefix(string, prefix):
    if string.startswith(prefix):
        return string
    return prefix + string


def trim_suffix(string, suffix):
    if string.endswith(suffix):
        return string[: -len(suffix)]
    return string


def ensure_suffix(string, suffix):
    if string.endswith(suffix):
        return string
    return string + suffix


def get_leftmost_part_of_path(path: str) -> str:
    """
    Gets the leftmost part of a path.

    If a path looks like
    /models/tensorflow/iris/15559399

    Then this function will return
    /models/
    """
    has_leading_slash = False
    if path.startswith("/"):
        path = path[1:]
        has_leading_slash = True

    basename = ""
    while path:
        path, basename = os.path.split(path)

    return "/" * has_leading_slash + basename


def remove_non_empty_directory_paths(paths: List[str]) -> List[str]:
    """
    Eliminates dir paths from the tree that are not empty.

    If paths looks like:
    models/tensorflow/
    models/tensorflow/iris/1569001258
    models/tensorflow/iris/1569001258/saved_model.pb

    Then after calling this function, it will look like:
    models/tensorflow/iris/1569001258/saved_model.pb
    """
    new_paths = []

    split_paths = [list(filter(lambda x: x != "", path.split("/"))) for path in paths]
    create_set_from_list = lambda l: set([(idx, split) for idx, split in enumerate(l)])
    split_set_paths = [create_set_from_list(split_path) for split_path in split_paths]

    for id_a, a in enumerate(split_set_paths):
        matches = 0
        for id_b, b in enumerate(split_set_paths):
            if id_a == id_b:
                continue
            if a.issubset(b):
                matches += 1
        if matches == 0:
            new_paths.append(paths[id_a])

    return new_paths


def merge_dicts_in_place_overwrite(*dicts):
    """Merge dicts, right into left, with overwriting. First dict is updated in place"""
    dicts = list(dicts)
    target = dicts.pop(0)
    for d in dicts:
        merge_two_dicts_in_place_overwrite(target, d)
    return target


def merge_dicts_in_place_no_overwrite(*dicts):
    """Merge dicts, right into left, without overwriting. First dict is updated in place"""
    dicts = list(dicts)
    target = dicts.pop(0)
    for d in dicts:
        merge_two_dicts_in_place_no_overwrite(target, d)
    return target


def merge_dicts_overwrite(*dicts):
    """Merge dicts, right into left, with overwriting. A new dict is created, original ones not modified."""
    result = {}
    for d in dicts:
        result = merge_two_dicts_overwrite(result, d)
    return result


def merge_dicts_no_overwrite(*dicts):
    """Merge dicts, right into left, without overwriting. A new dict is created, original ones not modified."""
    result = {}
    for d in dicts:
        result = merge_two_dicts_no_overwrite(result, d)
    return result


def merge_two_dicts_in_place_overwrite(x, y):
    """Merge y into x, with overwriting. x is updated in place"""
    if x is None:
        x = {}

    if y is None:
        y = {}

    for k, v in y.items():
        if k in x and isinstance(x[k], dict) and isinstance(y[k], collections.Mapping):
            merge_dicts_in_place_overwrite(x[k], y[k])
        else:
            x[k] = y[k]
    return x


def merge_two_dicts_in_place_no_overwrite(x, y):
    """Merge y into x, without overwriting. x is updated in place"""
    for k, v in y.items():
        if k in x and isinstance(x[k], dict) and isinstance(y[k], collections.Mapping):
            merge_dicts_in_place_no_overwrite(x[k], y[k])
        else:
            if k not in x:
                x[k] = y[k]
    return x


def merge_two_dicts_overwrite(x, y):
    """Merge y into x, with overwriting. A new dict is created, original ones not modified."""
    x = deepcopy(x)
    return merge_dicts_in_place_overwrite(x, y)


def merge_two_dicts_no_overwrite(x, y):
    """Merge y into x, without overwriting. A new dict is created, original ones not modified."""
    y = deepcopy(y)
    return merge_dicts_in_place_overwrite(y, x)


def is_bool(var):
    return isinstance(var, bool)


def is_float(var):
    return isinstance(var, float)


def is_int(var):
    return isinstance(var, int) and not isinstance(var, bool)


def is_str(var):
    return isinstance(var, str)


def is_dict(var):
    return isinstance(var, dict)


def is_list(var):
    return isinstance(var, list)


def is_tuple(var):
    return isinstance(var, tuple)


def is_float_or_int(var):
    return is_int(var) or is_float(var)


def is_int_list(var):
    if not is_list(var):
        return False
    for item in var:
        if not is_int(item):
            return False
    return True


def is_float_list(var):
    if not is_list(var):
        return False
    for item in var:
        if not is_float(item):
            return False
    return True


def is_str_list(var):
    if not is_list(var):
        return False
    for item in var:
        if not is_str(item):
            return False
    return True


def is_bool_list(var):
    if not is_list(var):
        return False
    for item in var:
        if not is_bool(item):
            return False
    return True


def is_float_or_int_list(var):
    if not is_list(var):
        return False
    for item in var:
        if not is_float_or_int(item):
            return False
    return True


def render_jinja_template(jinja_template_file: str, context: dict) -> str:
    from jinja2 import Environment, FileSystemLoader

    template_path = pathlib.Path(jinja_template_file)

    env = Environment(loader=FileSystemLoader(str(template_path.parent)))
    env.trim_blocks = True
    env.lstrip_blocks = True
    env.rstrip_blocks = True

    template = env.get_template(str(template_path.name))
    return template.render(**context)
