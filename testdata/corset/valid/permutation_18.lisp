;; Fragmented permutation with two targets in the same module as the source.
;; No trace can be accepted, since the targets together always hold twice as
;; many rows as the source.
(defcolumns (X :i16) (Y :i16) (Z :i16) (one :i1 :constant 1))
(defpermutation test ((one X)) ((one Y) (one Z)))
