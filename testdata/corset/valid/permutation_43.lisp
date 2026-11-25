;; As permutation_12, but without selectors.
(defconst ONE 1)
(defcolumns (X :i16) (Y :i16) (Z :i16))
;; Z holds the value of the expression (* ONE X).
(defconstraint z_def () (== Z (* ONE X)))
(defpermutation test ((_ Z)) ((_ Y)))
