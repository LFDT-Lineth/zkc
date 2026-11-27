;; Permutation between pairs of columns, where the first source column is
;; always zero.
(defcolumns (A :i16) (B :i16) (X :i16) (Z :i16) (one :i1 :constant 1))
;; Z holds the value of the constant 0.
(defconstraint z_def () (== Z 0))
(defpermutation test ((one Z X)) ((one A B)))
