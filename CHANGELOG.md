# Changelog
All notable changes to this project will be documented in this file.

## 2.1.2 - 2015-04-03

### Added
- Nothing

### Deprecated
- Nothing

### Removed
- Nothing

### Fixed
- Simplify Call() for rate breaker, fixing a reset bug

## 2.1.1 - 2014-10-29

### Added
- Nothing

### Deprecated
- Nothing

### Removed
- Nothing

### Fixed
- Ensure the half opens counter resets when the breaker resets, or auto-resetting may not occur

## 2.1.0 - 2014-10-16

### Added
- Failure, Sucess counts and Error Rate is now calculated over a sliding window
- Number of buckets in the window and the time the window spans are tuneable

### Deprecated
- Nothing

### Removed
- Nothing

### Fixed
- A race condition in Call()

## 2.0.2 - 2014-10-13

### Added
- ResetCounters

### Deprecated
- Nothing

### Removed
- Nothing

### Fixed
- Nothing

## 2.0.1 - 2014-10-13

### Added
- Nothing

### Deprecated
- Nothing

### Removed
- Nothing

### Fixed
- Error rate should return 0.0 if there have been no samples

## 2.0.0 - 2014-10-13

### Added
- All circuit breakers are now a Breaker with trip semantics handled by a TripFunc
- NewConsecutiveBreaker
- NewRateBreaker
- ConsecFailures
- ErrorRate
- Success
- Successes
- Retry logic now uses cenkalti/backoff, exponential backoff by default

### Deprecated
- Nothing

### Removed
- TrippableBreaker, ThresholdBreaker, FrequencyBreaker, TimeoutBreaker; all handled by Breaker now
- NewFrequencyBreaker, replaced by NewConsecutiveBreaker
- NewTimeoutBreaker, time out semantics are now handled by Call()
- NoOp(), use a Breaker with no TripFunc instead

### Fixed
- Nothing

## 1.1.2 - 2014-08-20

### Added
- Nothing

### Deprecated
- Nothing

### Fixed
- For a FrequencyBreaker, Failures() should return the count since the duration start, even after resetting.

## 1.1.1 - 2014-08-20

### Added
- Nothing

### Deprecated
- Nothing

### Fixed
- Only send the reset event if the breaker was in a tripped state

## 1.1.0 - 2014-08-16

### Added
- Re-export a Panels Circuits map. It's handy and if you mess it up, it's on you.

### Deprecated
- Nothing

### Removed
- Nothing

### Fixed
- Nothing

## 1.0.0 - 2014-08-16

### Added
- This will be the public API for version 1.0.0. This project will follow semver rules.
