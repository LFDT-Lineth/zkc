;; As permutation_18, but where only one target has a selector.  Again, no
;; trace can be accepted.
(defcolumns (X :i16) (Y :i16) (Z :i16) (one :i1 :constant 1))
(defpermutation test ((_ X)) ((_ Y) (one Z)))
