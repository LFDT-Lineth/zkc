;; As permutation_06, but without selectors.
(module m1)
(defcolumns (X :i16))

(module m2)
(defcolumns (Y :i16))
(defpermutation test ((_ m1.X)) ((_ Y)))
