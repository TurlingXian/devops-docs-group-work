"""
Differential Privacy DSL Implementation

This module provides the main DP_DSL class that students need to implement.
The noise generation methods are provided, but students must implement:
- Budget management
- Data transformations
- DP measurements
- Stability tracking

STUDENT TODO: Complete the implementation of the DP_DSL class.
"""

from typing import TypeVar, Callable, List, Tuple, Dict, Optional, Any
from collections import defaultdict
import numpy as np
import math

from core import Dataset, ConcreteDataset, DPResult, InsufficientBudget, InvalidParameter, Epsilon
from adult_dataset import Adult

T = TypeVar('T')
U = TypeVar('U')
K = TypeVar('K')


class DP_DSL:
    """Unified Differential Privacy DSL

    This class encapsulates privacy budget management, transformations, and measurements
    in a single interface for ease of use.

    STUDENT TODO: Complete the implementation of this class.

    Key concepts you need to implement:
    1. Budget Management: Track epsilon consumption and prevent overspending
    2. Transformations: Implement filter, map, union, intersect, group_by with proper stability tracking
    3. Measurements: Implement count and sum with proper noise addition
    4. Stability Tracking: Ensure transformations correctly update dataset stability
    """

    def __init__(self, budget: float):
        """Initialize DSL with privacy budget

        Args:
            budget: Total privacy budget available for this session

        """
        if budget <=0:
            raise InvalidParameter(f"A budget must be larger than 0, got {budget}")
        self._total = budget
        self._used = 0

    # ============================================================================
    # Budget Management Methods (STUDENT TODO)
    # ============================================================================

    def get_remaining_budget(self) -> float:
        """Get remaining privacy budget

        STUDENT TODO: Return the remaining budget (total - used)
        """
        return (self._total - self._used)

    def get_used_budget(self) -> float:
        """Get used privacy budget

        STUDENT TODO: Return the amount of budget that has been consumed
        """
        return self._used

    def get_total_budget(self) -> float:
        """Get total privacy budget

        STUDENT TODO: Return the total budget allocated to this DSL instance
        """
        return self._total

    def _consume_budget(self, epsilon: float) -> None:
        """Internal method to consume privacy budget

        Args:
            epsilon: Privacy parameter to spend

        Raises:
            InvalidParameter: If epsilon is not positive
            InsufficientBudget: If insufficient budget remains

        """
        if epsilon <= 0:
            raise InvalidParameter(f"Epsilon must be positive, got {epsilon}")

        remaining = self.get_remaining_budget()
        
        if epsilon > remaining:
            raise InsufficientBudget(epsilon, remaining)

        self._used += epsilon



    # ============================================================================
    # Data Transformations (Budget-Free) - STUDENT TODO
    # ============================================================================

    def filter(self, predicate: Callable[[T], bool], dataset: Dataset[T]) -> Dataset[T]:
        """Filter dataset while preserving stability

        Args:
            predicate: Function to filter elements
            dataset: Input dataset

        Returns:
            Filtered dataset with same stability

        """
        filtered_data = [item for item in dataset if predicate(item)]
        stb = dataset.stability
    
        return dataset._create_new(filtered_data, stb)
    
    def map(self, func: Callable[[T], U], dataset: Dataset[T]) -> Dataset[U]:
        """Map function over dataset while preserving stability

        Args:
            func: Function to apply to each element
            dataset: Input dataset

        Returns:
            Mapped dataset with same stability

        """
        mapped_data = [func(item) for item in dataset]
        stb = dataset.stability

        return dataset._create_new(mapped_data, stb)
        

    def union(self, dataset1: Dataset[T], dataset2: Dataset[T]) -> Dataset[T]:
        """Union two datasets with stability amplification

        Args:
            dataset1: First dataset
            dataset2: Second dataset

        Returns:
            Combined dataset with amplified stability

        """
        union_dataset = list(dataset1) + list(dataset2)
        union_stability = dataset1.stability + dataset2.stability

        return dataset1._create_new(union_dataset, union_stability)

    def intersect(self, dataset1: Dataset[T], dataset2: Dataset[T]) -> Dataset[T]:
        """Intersect two datasets with stability amplification

        Args:
            dataset1: First dataset
            dataset2: Second dataset

        Returns:
            Intersection dataset with amplified stability

        STUDENT TODO: Implement intersection transformation.
        """
        d2_items = list(dataset2)
        intersection_dataset = [item for item in dataset1 if item in d2_items]

        intersection_stability = dataset1.stability + dataset2.stability

        return dataset1._create_new(intersection_dataset, intersection_stability)

    def group_by(self, key_func: Callable[[T], K], dataset: Dataset[T]) -> Dataset[Tuple[K, List[T]]]:
        """Group dataset by key function (stability doubles)

        Args:
            key_func: Function to extract grouping key from each element
            dataset: Input dataset

        Returns:
            Dataset of (key, group) pairs with doubled stability

        """
        groups: Dict[K, List[T]] = {}
        for item in dataset:
            key = key_func(item)
            if key not in groups:
                groups[key] = []
            groups[key].append(item)

        groupped_dataset = list(groups.items())
        groupped_stability = 2 * dataset.stability

        return dataset._create_new(groupped_dataset, groupped_stability)

    def partition(self,
                  keys: List[K],
                  classifier: Callable[[T], K],
                  dataset: Dataset[T],
                  computation: Callable[[K, Dataset[T]], Any],
                  epsilon: Epsilon) -> List[Tuple[K, Any]]:
        """Partition dataset and apply computation to each partition

        Args:
            keys: List of expected partition keys
            classifier: Function to classify elements into partitions
            dataset: Input dataset
            computation: Function to apply to each partition
            epsilon: Privacy budget to spend on this operation

        Returns:
            List of (key, result) pairs for each partition

        """
        self._consume_budget(epsilon)
        buckets = {k: [] for k in keys}

        for item in dataset:
            k = classifier(item)
            if k in buckets:
                buckets[k].append(item)
        
        results = []
        prev_stability = dataset.stability

        final_used_budget = self._used

        for k in keys:
            partition_data = buckets[k]
            partition_dataset = dataset._create_new(partition_data, prev_stability)

            self._used -= epsilon

            res = computation(k, partition_dataset)

            self._used = final_used_budget
            results.append((k, res))

        return results


    # ============================================================================
    # Differentially Private Measurements (Budget-Consuming) - STUDENT TODO
    # ============================================================================

    def count(self, dataset: Dataset[T], epsilon: Epsilon) -> DPResult:
        """Differentially private count measurement

        Args:
            dataset: Dataset to count
            epsilon: Privacy parameter

        Returns:
            DPResult with noisy count and epsilon spent

        """
        self._consume_budget(epsilon)

        true_count = len(dataset)

        sensitivity = 1 * dataset.stability # sen of count is 1

        res = self._add_laplace_noise(true_count, sensitivity, epsilon)

        return DPResult(res, epsilon)

    def sum(self,
            dataset: Dataset[float],
            epsilon: Epsilon,
            lower_bound: float,
            upper_bound: float) -> DPResult:
        """Differentially private sum measurement with bounded contributions

        Args:
            dataset: Dataset of numeric values to sum
            epsilon: Privacy parameter
            lower_bound: Lower bound on individual contributions
            upper_bound: Upper bound on individual contributions

        Returns:
            DPResult with noisy sum and epsilon spent

        """
        self._consume_budget(epsilon)

        true_sum = 0.0
        for item in dataset:
            true_sum += min(upper_bound, max(lower_bound, item)) # capped the data, must be usable
        
        max_magnitude = max(abs(lower_bound), abs(upper_bound))
        sensitivity = dataset.stability * max_magnitude

        res = self._add_laplace_noise(true_sum, sensitivity, epsilon)

        return DPResult(res, epsilon)

    # ============================================================================
    # Noise Generation (PROVIDED - DO NOT MODIFY)
    # ============================================================================

    def _laplace_noise(self, scale: float) -> float:
        """Generate noise from the Laplace distribution

        Args:
            scale: Scale parameter (must be positive)

        Returns:
            Random sample from Laplace distribution

        Raises:
            ValueError: If scale is not positive
        """
        if scale <= 0:
            raise ValueError(f"Scale must be positive, got {scale}")

        return np.random.laplace(loc=0, scale=scale)

    def _add_laplace_noise(self, value: float, sensitivity: float, epsilon: float) -> float:
        """Add calibrated Laplace noise to a value

        This method is provided for you. The noise scale is sensitivity/epsilon,
        which ensures ε-differential privacy.

        Args:
            value: The true value to add noise to
            sensitivity: The sensitivity of the query
            epsilon: Privacy parameter

        Returns:
            Value with added noise
        """
        noise = self._laplace_noise(sensitivity / epsilon)
        return value + noise

    # ============================================================================
    # Convenience Methods (STUDENT TODO)
    # ============================================================================

    def reset_budget(self, new_budget: float) -> None:
        """Reset the privacy budget to a new value

        Args:
            new_budget: New total privacy budget

        STUDENT TODO: Reset both total and used budget.
        Validate that new_budget > 0.
        """
        if new_budget <= 0:
            raise ValueError(f"Expected new budget is positive, got {new_budget}")
        self._total = new_budget
        self._used = 0

    def budget_status(self) -> Dict[str, float]:
        """Get current budget status

        Returns:
            Dictionary with budget information
        """
        # TODO: Implement budget status reporting
        infor = {}
        infor["total_budget"] = self.get_total_budget()
        infor["used_budget"] = self.get_used_budget()
        infor["remaining_budget"] = self.get_remaining_budget()

        total = self.get_total_budget()
        if total > 0:
            infor["usage_percentage"] = (self.get_used_budget() / total) * 100.0
        else:
            infor["usage_percentage"] = 0.0

        return infor

    def __str__(self) -> str:
        """String representation of DSL state

        """
        stats = self.budget_status()
        return (f"DP_DSL Budget: {stats['used_budget']:.3f}/{stats['total_budget']:.3f} "
                f"({stats['usage_percentage']:.1f}%)")

    def __repr__(self) -> str:
        """Detailed representation of DSL state"""
        return self.__str__()


# ============================================================================
# Factory Functions
# ============================================================================

def create_dsl(budget: float) -> DP_DSL:
    """Create a new DP_DSL instance

    Args:
        budget: Total privacy budget

    Returns:
        New DP_DSL instance
    """
    return DP_DSL(budget)
