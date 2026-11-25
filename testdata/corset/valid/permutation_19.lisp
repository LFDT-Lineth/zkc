;; Fragmented permutation with two targets (of two columns each) in the same
;; module as the source.  No trace can be accepted, since the targets together
;; always hold twice as many rows as the source.
(defcolumns (X1 :i16) (X2 :i16) (Y1 :i16) (Y2 :i16) (Z1 :i16) (Z2 :i16) (one :i1 :constant 1))
(defpermutation test ((one X1 X2)) ((one Y1 Y2) (one Z1 Z2)))
