;; As permutation_19, but without selectors.  Again, no trace can be accepted.
(defcolumns (X1 :i16) (X2 :i16) (Y1 :i16) (Y2 :i16) (Z1 :i16) (Z2 :i16))
(defpermutation test ((_ X1 X2)) ((_ Y1 Y2) (_ Z1 Z2)))
