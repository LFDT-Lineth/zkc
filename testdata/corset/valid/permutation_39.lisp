;; As permutation_08, but without selectors.
(defcolumns (A :i16) (B :i16) (X :i16) (Z :i16))
;; Z holds the value of the constant 0.
(defconstraint z_def () (== Z 0))
(defpermutation test ((_ Z X)) ((_ A B)))
