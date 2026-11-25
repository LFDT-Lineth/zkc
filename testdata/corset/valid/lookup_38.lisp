;; As lookup_07, but without a target selector.
(defcolumns (A :i16) (B :i16) (X :i16) (Z :i16) (one :i1 :constant 1))
;; Z holds the value of the constant 0
(defconstraint z_def () (== Z 0))
(deflookup test ((one X Z)) ((_ A B)))
