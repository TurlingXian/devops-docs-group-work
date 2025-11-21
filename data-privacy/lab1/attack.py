"""
Reconstruction attack implementation as described by Nissim and Dinur.
"""

from itertools import chain, combinations, product
from typing import List, Tuple

from db import *

# A candidate is a guess on the `hiv` conditions of all the patients.
Candidate = List[HIV]

# A candidate with (public) patient names attached.
CandidateWithNames = List[Tuple[Name, HIV]]

# The result of our queries as a `float`, since we are releasing sums of `hiv` conditions with noise.
ResultQuery = float

def comb(k: int, ns: List[int]) -> List[List[int]]:
  """Generates the combination of elements in `ns` by taking groups of size `k`."""
  return list(map(list, combinations(ns, k)))

def indexes(db_size: int) -> List[List[Index]]:
  """Generates all possible combinations of indexes for a database of given `db_size`."""
  result = []
  indexes_list = []
  for i in range(db_size):
    indexes_list.append(i)
  
  # generate and append (a flatten list) all combination with k element(s), k from 1 to n
  for r in range(1, db_size + 1):
    result.extend(comb(r, indexes_list))
  return result

def all_sums(db: DB, noise: Noise) -> List[ResultQuery]:
  """Performs the noisy sums of all combinations of indexes for a given database `db`. It calls `add`."""
  result = []
  combination_list = indexes(db_size(db))

  for type_combs in combination_list:
    result.append(add(db, noise, type_combs))

  return result

def sum_indexes(candidate: Candidate, idx: List[Index]) -> ResultQuery:
  """Given a candidate and some indexes `idx`, it performs the sum of the conditions (without noise)."""
  result = 0

  for val in idx:
    currentCondition = candidate[val]
    result += currentCondition

  return result

def all_sums_no_noise(candidate: Candidate) -> List[ResultQuery]:
  """Given a candidate, it performs the sums (without noise) of all combinations of indexes."""
  result = []

  # generate the list of combinations base on the length of passed candidate
  list_of_combinations = indexes(len(candidate))
  for combins in list_of_combinations:
    current_sum = sum_indexes(candidate, combins)
    result.append(current_sum)

  return result

def generate_candidates(db: DB) -> List[Candidate]:
  """Given a database `db`, it generates all the possible candidates."""
  result = []
  total_space = db_size(db=db)

  possible_combinations = product([0.0, 1.0], repeat=total_space) # 0 0 1 1 0 0, 0 1 1 0 0 0 ...
  for combns in possible_combinations: #
    result.append(combns)

  return result

def fit(noise_mag: Noise, results: List[ResultQuery], candidate: Candidate) -> bool:
  """
  This function will determine if exists a non-noisy sum on the candidate and a
  corresponding noisy sum in `results` whose distance is greater than `noise_mag`.
  """

  no_noisy_sum = all_sums_no_noise(candidate)
  if len(no_noisy_sum) != len(results):
    raise ValueError("The length of noisy and no noisy are not the same")
  
  for noisy_record, no_noisy_record in zip(results, no_noisy_sum):
    distance = abs(noisy_record - no_noisy_record)
  
    if distance > noise_mag:
      return False

  return True

def find_candidates(db: DB, noise: Noise) -> List[Candidate]:
  """Finds candidates whose non-noisy sums "fit" the noisy ones."""
  result = []

  list_of_candidate = generate_candidates(db)
  sums_with_noise = all_sums(db, noise)

  for candidates in list_of_candidate:
    if fit(noise, sums_with_noise, candidates):
      result.append(candidates)

  return result

def attack(db: DB, noise: Noise) -> List[CandidateWithNames]:
  """Guess the conditions of all patients in the dataset."""
  result = []

  possible_candidates = find_candidates(db, noise)
  all_names = names(db)

  for possibility in possible_candidates:
    current_guess = []
    for name_record, hiv_record in zip(all_names, possibility):
      current_record = (name_record, hiv_record)
      current_guess.append(current_record)
    result.append(current_guess)
  return result
